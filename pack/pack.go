package pack

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
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

	return &PackFile{Version: PackFileVersion, Entries: entries}, nil

}

func Write(w io.Writer, pack *PackFile) error {
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

	entry, err := encodeEntryList(pack.Entries, 3+4+len(dummyEntry))
	if err != nil {
		return err
	}

	err = binary.Write(w, endian, uint32(len(entry)))
	if err != nil {
		return err
	}

	_, err = w.Write(entry)
	if err != nil {
		return err
	}

	return nil
}

// Pack バケットからPackファイルを作成する
/*
func Pack(d *Downloader, b *Bucket) ([]byte, error) {
	for _, c := range b.Contents {

		data, err := d.Fetch(c.Hash, c.Attr)
		if err != nil {
			return nil, err
		}

		_ = data

	}
	return nil, nil
}

func writeToPack(w io.Writer, b *Bucket, f func(string) []byte) error {
	return nil
}
*/

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

	for _, e := range entries {
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
