package pack

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/text/unicode/norm"
	"local.package/cfs"
)

// PackFile パックファイルを表す
type PackFile struct {
	Version int
	Entries []Entry
}

// Entry パックファイルの中の一つのファイルを表す
type Entry struct {
	Path string
	Hash string
	Pos  int // Pos は、Write時に書き込まれるので設定不要
	Size int
	Data []byte
}

// PackFileVersion は現在のPackファイルのバージョン
const PackFileVersion = 1

// 標準的に使用するエンディアン
var endian = binary.LittleEndian

// NewPackFile PackFileを新規に作成する
func NewPackFile(entries []Entry) *PackFile {
	return &PackFile{Version: PackFileVersion, Entries: entries}
}

// Parse PackファイルをParseする
func Parse(r io.Reader) (*PackFile, error) {
	err := decodeHeader(r)
	if err != nil {
		return nil, err
	}

	var entrySize uint32
	err = binary.Read(r, endian, &entrySize)
	if err != nil {
		return nil, err
	}

	entryList := make([]byte, entrySize)
	_, err = io.ReadFull(r, entryList[:])
	if err != nil {
		return nil, err
	}

	entries, err := decodeEntryList(entryList, int(entrySize))
	if err != nil {
		return nil, err
	}

	for i := range entries {
		e := &entries[i]
		e.Data = make([]byte, e.Size)
		len, err := r.Read(e.Data)
		if len != e.Size {
			return nil, fmt.Errorf("invalid read size")
		}
		if err != nil {
			return nil, err
		}
	}

	return &PackFile{Version: PackFileVersion, Entries: entries}, nil

}

// Pack PackFileをファイルに書き込む
func Pack(w io.Writer, pack *PackFile, fn func(string) io.Reader) error {
	_, err := w.Write(encodeHeader())
	if err != nil {
		return err
	}

	// Entryをソートする
	sort.Slice(pack.Entries, func(i, j int) bool { return pack.Entries[i].Path < pack.Entries[j].Path })

	// EntryListのサイズを取得する
	dummyEntry, err := encodeEntryList(pack.Entries, 0)
	if err != nil {
		return err
	}

	err = binary.Write(w, endian, uint32(len(dummyEntry)))
	if err != nil {
		return err
	}

	entry, err := encodeEntryList(pack.Entries, 3+4+len(dummyEntry))
	if err != nil {
		return err
	}

	_, err = w.Write(entry)
	if err != nil {
		return err
	}

	if fn == nil {
		for _, e := range pack.Entries {
			if e.Data == nil {
				return fmt.Errorf("invalid data in %s", e.Path)
			}
			if e.Size > 0 {
				size, err := w.Write(e.Data)
				if err != nil {
					return err
				}
				if int(size) != e.Size {
					return fmt.Errorf("invalid written size %s, expect %d but %d", e.Path, e.Size, size)
				}
			}
		}
	} else {
		for _, e := range pack.Entries {
			fr := fn(e.Path)
			size, err := io.Copy(w, fr)
			if err != nil || int(size) != e.Size {
				return err
			}
		}
	}

	return nil
}

// NewPackFileFromDir ディレクトリを指定して、パックファイルを作成する
func NewPackFileFromDir(dir string) (*PackFile, error) {
	entries := []Entry{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		// OSXのためにUTF-8文字列を正規化する See: https://text.baldanders.info/golang/unicode-normalization/
		path = norm.NFC.String(path)
		if !info.IsDir() {
			entryPath := filepath.ToSlash(path[len(dir)+1:])

			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			hash := fmt.Sprintf("%x", md5.Sum(data))

			entries = append(entries, Entry{
				Path: entryPath,
				Hash: hash,
				Size: int(info.Size()),
				Data: data,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &PackFile{Version: PackFileVersion, Entries: entries}, nil
}

// Patch パッチを作成する
func Patch(base, current *PackFile) (*PackFile, error) {
	// Make base entries map by path.
	baseEntryMap := map[string]Entry{}
	for _, e := range base.Entries {
		baseEntryMap[e.Path] = e
	}

	// Make diff from base to current.
	entries := []Entry{}
	for _, e := range current.Entries {
		baseEntry, found := baseEntryMap[e.Path]
		same := false
		if found {
			//fmt.Printf("%v %v %v\n", e.Path, e.Hash, baseEntry.Hash)
			if baseEntry.Hash == e.Hash {
				same = true
			} else {
				if cfs.Verbose {
					fmt.Printf("not same file path:%v, base-hash:%v, current-hash:%v\n", e.Path, baseEntry.Hash, e.Hash)
				}
			}
		} else {
			if cfs.Verbose {
				fmt.Printf("new file %v\n", e.Path)
			}
		}

		if !same {
			entries = append(entries, e)
		} else {
			//fmt.Printf("same %v\n", e.Path)
		}
	}

	return &PackFile{Version: PackFileVersion, Entries: entries}, nil
}

func encodeHeader() []byte {
	return []byte{byte('T'), byte('P'), PackFileVersion}
}

func encodeEntryList(entries []Entry, bodyPos int) ([]byte, error) {
	w := bytes.NewBuffer(nil)
	pos := bodyPos

	err := binary.Write(w, endian, uint32(len(entries)))
	if err != nil {
		return nil, err
	}

	for i, e := range entries {
		e.Pos = int(pos)
		entries[i].Pos = int(pos)
		binary.Write(w, endian, byte(len(e.Path)))
		w.Write([]byte(e.Path))
		binary.Write(w, endian, uint32(e.Pos))
		binary.Write(w, endian, uint32(e.Size))
		hashBytes, err := hex.DecodeString(e.Hash)
		if err != nil {
			return nil, err
		}
		w.Write(hashBytes)
		pos += e.Size
	}

	return w.Bytes(), nil
}

func decodeHeader(r io.Reader) error {
	var header [3]byte
	_, err := io.ReadFull(r, header[:])
	if err != nil {
		return err
	}

	if header[0] != byte('T') || header[1] != byte('P') {
		return fmt.Errorf("Invalid file header, magic")
	}
	if header[2] != PackFileVersion {
		return fmt.Errorf("Invalid file header, version")
	}
	return nil
}

func decodeEntryList(bin []byte, entrySize int) ([]Entry, error) {
	r := bytes.NewBuffer(bin)

	var entryCount uint32
	err := binary.Read(r, endian, &entryCount)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, entryCount)

	for i := 0; i < int(entryCount); i++ {
		var pathLen byte
		var pathBytes [256]byte
		var pos uint32
		var size uint32
		var hash [16]byte

		err := binary.Read(r, endian, &pathLen)
		if err != nil {
			return nil, err
		}

		_, err = io.ReadFull(r, pathBytes[:pathLen])
		if err != nil {
			return nil, err
		}

		err = binary.Read(r, endian, &pos)
		if err != nil {
			return nil, err
		}

		err = binary.Read(r, endian, &size)
		if err != nil {
			return nil, err
		}

		_, err = io.ReadFull(r, hash[:])
		if err != nil {
			return nil, err
		}

		entries[i] = Entry{
			Path: string(pathBytes[:pathLen]),
			Hash: hex.EncodeToString(hash[:]),
			Pos:  int(pos),
			Size: int(size),
		}
	}

	return entries, nil
}
