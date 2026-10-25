package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/text/unicode/norm"
)

// PackFile パックファイルを表す
type PackFile struct {
	Version int
	Entries []Entry
	Deleted []string // パッチで消すファイルのパス（Patch が作る。ふつうのパックでは空）
}

// Entry パックファイルの中の一つのファイルを表す
type Entry struct {
	Path string
	Hash string
	Pos  int // Pos は、Write時に書き込まれるので設定不要
	Size int
	Data []byte
}

// PackFileVersion は現在のPackファイルのバージョン（3 で、消すファイルの一覧を加えた）
const PackFileVersion = 3

// maxPathLen は、パスの最大のバイト数（長さを 1 バイトで書くため）
const maxPathLen = 255

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

	entries, deleted, err := decodeEntryList(entryList)
	if err != nil {
		return nil, err
	}

	for i := range entries {
		e := &entries[i]
		e.Data = make([]byte, e.Size)
		if _, err := io.ReadFull(r, e.Data); err != nil {
			return nil, fmt.Errorf("invalid read size of %s: %w", e.Path, err)
		}
	}

	return &PackFile{Version: PackFileVersion, Entries: entries, Deleted: deleted}, nil
}

// Pack PackFileをファイルに書き込む
func Pack(w io.Writer, pack *PackFile, fn func(string) io.Reader) error {
	_, err := w.Write(encodeHeader())
	if err != nil {
		return err
	}

	// Entryをソートする
	sort.Slice(pack.Entries, func(i, j int) bool { return pack.Entries[i].Path < pack.Entries[j].Path })
	sort.Strings(pack.Deleted)

	// EntryListのサイズを取得する
	dummyEntry, err := encodeEntryList(pack.Entries, pack.Deleted, 0)
	if err != nil {
		return err
	}

	err = binary.Write(w, endian, uint32(len(dummyEntry)))
	if err != nil {
		return err
	}

	entry, err := encodeEntryList(pack.Entries, pack.Deleted, 3+4+len(dummyEntry))
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
			if err != nil {
				return err
			}
			if int(size) != e.Size {
				return fmt.Errorf("invalid written size %s, expect %d but %d", e.Path, e.Size, size)
			}
		}
	}

	return nil
}

// NewPackFileFromDir ディレクトリを指定して、パックファイルを作成する
func NewPackFileFromDir(dir string) (*PackFile, error) {
	entries := []Entry{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// "." や末尾に区切りのあるパスでも正しくなるように、相対パスで求める
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			// OSXのためにUTF-8文字列を正規化する See: https://text.baldanders.info/golang/unicode-normalization/
			entryPath := norm.NFC.String(filepath.ToSlash(rel))

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(data))

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

// Patch は、base から current への差分のパックを作る。
// 増えたものと変わったものを中身に、base にあって current にないものを Deleted に入れる
func Patch(base, current *PackFile) (*PackFile, error) {
	baseHashes := map[string]string{}
	for _, e := range base.Entries {
		baseHashes[e.Path] = e.Hash
	}
	currentPaths := map[string]bool{}
	for _, e := range current.Entries {
		currentPaths[e.Path] = true
	}

	entries := []Entry{}
	for _, e := range current.Entries {
		if hash, found := baseHashes[e.Path]; !found || hash != e.Hash {
			entries = append(entries, e)
		}
	}
	deleted := []string{}
	for _, e := range base.Entries {
		if !currentPaths[e.Path] {
			deleted = append(deleted, e.Path)
		}
	}
	sort.Strings(deleted)

	return &PackFile{Version: PackFileVersion, Entries: entries, Deleted: deleted}, nil
}

func encodeHeader() []byte {
	return []byte{byte('T'), byte('P'), PackFileVersion}
}

// writePath は、パスを長さ（1 バイト）と UTF-8 のバイト列で書く
func writePath(w *bytes.Buffer, p string) error {
	if len(p) > maxPathLen {
		return fmt.Errorf("path is too long (max %d bytes): %s", maxPathLen, p)
	}
	w.WriteByte(byte(len(p)))
	w.WriteString(p)
	return nil
}

// readPath は、writePath で書いたパスを読む
func readPath(r *bytes.Buffer) (string, error) {
	pathLen, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return "", err
	}
	return string(pathBytes), nil
}

func encodeEntryList(entries []Entry, deleted []string, bodyPos int) ([]byte, error) {
	w := bytes.NewBuffer(nil)
	pos := bodyPos

	err := binary.Write(w, endian, uint32(len(entries)))
	if err != nil {
		return nil, err
	}

	for i, e := range entries {
		e.Pos = int(pos)
		entries[i].Pos = int(pos)
		if err := writePath(w, e.Path); err != nil {
			return nil, err
		}
		binary.Write(w, endian, uint32(e.Pos))
		binary.Write(w, endian, uint32(e.Size))
		hashBytes, err := hex.DecodeString(e.Hash)
		if err != nil {
			return nil, err
		}
		w.Write(hashBytes)
		pos += e.Size
	}

	// 消すファイルの一覧
	if err := binary.Write(w, endian, uint32(len(deleted))); err != nil {
		return nil, err
	}
	for _, p := range deleted {
		if err := writePath(w, p); err != nil {
			return nil, err
		}
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
		return fmt.Errorf("unsupported pack file version %d (supported: %d)", header[2], PackFileVersion)
	}
	return nil
}

func decodeEntryList(bin []byte) ([]Entry, []string, error) {
	r := bytes.NewBuffer(bin)

	var entryCount uint32
	err := binary.Read(r, endian, &entryCount)
	if err != nil {
		return nil, nil, err
	}

	entries := make([]Entry, entryCount)

	for i := 0; i < int(entryCount); i++ {
		var pos uint32
		var size uint32
		var hash [sha256.Size]byte

		path, err := readPath(r)
		if err != nil {
			return nil, nil, err
		}

		err = binary.Read(r, endian, &pos)
		if err != nil {
			return nil, nil, err
		}

		err = binary.Read(r, endian, &size)
		if err != nil {
			return nil, nil, err
		}

		_, err = io.ReadFull(r, hash[:])
		if err != nil {
			return nil, nil, err
		}

		entries[i] = Entry{
			Path: path,
			Hash: hex.EncodeToString(hash[:]),
			Pos:  int(pos),
			Size: int(size),
		}
	}

	var deletedCount uint32
	if err := binary.Read(r, endian, &deletedCount); err != nil {
		return nil, nil, err
	}
	deleted := make([]string, 0, deletedCount)
	for i := 0; i < int(deletedCount); i++ {
		p, err := readPath(r)
		if err != nil {
			return nil, nil, err
		}
		deleted = append(deleted, p)
	}

	return entries, deleted, nil
}
