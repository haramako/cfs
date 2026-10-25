package cfs

import (
	"fmt"
	"sync"
)

type DummyStorage struct {
	CabinetPath string
	mutex       sync.Mutex // Upload は複数のワーカーから並列に呼ばれる
	contents    map[string][]byte
	tags        map[string][]byte

	onUpload func(filename string, hash string, body []byte, overwrite bool) error
}

func NewDummyStorage(cabinetPath string) (*DummyStorage, error) {
	s := &DummyStorage{
		CabinetPath: cabinetPath,
		contents:    make(map[string][]byte),
		tags:        make(map[string][]byte),
	}

	return s, nil
}

func (s *DummyStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {
	if !isHash(hash) {
		return fmt.Errorf("%v is not hash", hash)
	}

	if s.onUpload != nil {
		err := s.onUpload(filename, hash, body, overwrite)
		if err != nil {
			return err
		}
	}

	s.mutex.Lock()
	s.contents[hash] = body
	s.mutex.Unlock()

	if Verbose {
		fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	}

	return nil
}

func (s *DummyStorage) UploadTag(filename string, body []byte) error {
	s.mutex.Lock()
	s.tags[filename] = body
	s.mutex.Unlock()

	if Verbose {
		fmt.Printf("uploading tag '%s'\n", filename)
	}

	return nil
}
