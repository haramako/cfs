package pack

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

// PackFile パックファイルを表す
type packFile struct {
	Version      int
	Entries      []packEntry
	BodyPosition int
}

type packEntry struct {
	path string
	hash string
	pos  int
	size int
}

func Unpack(r io.Reader) (*packFile, error) {
	return nil, nil

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

func encodePackHeader(entries []packEntry) []byte {
	return []byte{byte('T'), byte('P'), 1}
}

func encodePackEntryList(entries []packEntry) ([]byte, error) {
	s := bytes.NewBuffer(nil)
	endian := binary.LittleEndian

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

func decodePackHeader(b []byte) error {
	if b[0] != byte('T') || b[1] != byte('P') {
		return fmt.Errorf("Invalid file header, magic")
	}
	if b[2] != 1 {
		return fmt.Errorf("Invalid file header, version")
	}
	return nil
}

func decodePackEntryList(b []byte, entryCount int) ([]packEntry, error) {
	s := bytes.NewBuffer(b)
	endian := binary.LittleEndian

	entries := make([]packEntry, 0, entryCount)

	for i := 0; i < entryCount; i++ {
		var pathLen byte
		var pathBytes [256]byte
		var pos uint32
		var size uint32
		var hash [16]byte

		err := binary.Read(s, endian, &pathLen)
		if err != nil {
			return nil, err
		}

		_, err = s.Read(pathBytes[:pathLen])
		if err != nil {
			return nil, err
		}

		err = binary.Read(s, endian, &pos)
		if err != nil {
			return nil, err
		}

		err = binary.Read(s, endian, &size)
		if err != nil {
			return nil, err
		}

		_, err = s.Read(hash[:])
		if err != nil {
			return nil, err
		}

		entries = append(entries, packEntry{
			path: string(pathBytes[:pathLen]),
			hash: hex.EncodeToString(hash[:]),
			pos:  int(pos),
			size: int(size),
		})
	}

	return entries, nil
}
