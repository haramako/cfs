package cfs

import (
	"fmt"
	"net/url"
)

type DummyStorage struct {
	CabinetPath string
	rootUrl     *url.URL
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

func (s *DummyStorage) DownloaderUrl() *url.URL {
	return s.rootUrl
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

	s.contents[hash] = body

	if Verbose {
		fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	}

	return nil
}

func (s *DummyStorage) UploadTag(filename string, body []byte) error {
	s.tags[filename] = body

	if Verbose {
		fmt.Printf("uploading tag '%s'\n", filename)
	}

	return nil
}
