package pack

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

// PackFile パックファイルを表す
type PackFile struct {
	Version int
	Entries []Entry
}

type Entry struct {
	path string
	hash string
	pos  int
	size int
}

// PackFileVersion は現在のPackファイルのバージョン
const PackFileVersion = 1

// 標準的に使用するエンディアン
var endian = binary.LittleEndian

// Unpack
func Unpack(r io.Reader) (*PackFile, error) {
	err := decodeHeader(r)
	if err != nil {
		return nil, err
	}

	var entrySize uint32
	err = binary.Read(r, endian, &entrySize)
	if err != nil {
		return nil, err
	}

	entries, err := decodeEntryList(r, int(entrySize))
	if err != nil {
		return nil, err
	}

	return &PackFile{Version: PackFileVersion, Entries: entries}, nil

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

func encodeHeader(entries []Entry) []byte {
	return []byte{byte('T'), byte('P'), PackFileVersion}
}

func encodePackEntryList(entries []Entry) ([]byte, error) {
	s := bytes.NewBuffer(nil)

	for _, e := range entries {
		binary.Write(s, endian, byte(len(e.path)))
		s.Write([]byte(e.path))
		binary.Write(s, endian, uint32(e.pos))
		binary.Write(s, endian, uint32(e.size))
		hashBytes, err := hex.DecodeString(e.hash)
		if err != nil {
			return nil, err
		}
		s.Write(hashBytes)
	}

	buf := s.Bytes()

	return buf, nil
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

func decodeEntryList(r io.Reader, entryCount int) ([]Entry, error) {
	entries := make([]Entry, 0, entryCount)

	for i := 0; i < entryCount; i++ {
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

		entries = append(entries, Entry{
			path: string(pathBytes[:pathLen]),
			hash: hex.EncodeToString(hash[:]),
			pos:  int(pos),
			size: int(size),
		})
	}

	return entries, nil
}
