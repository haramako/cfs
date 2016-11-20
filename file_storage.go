package cfs

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
)

type FileStorage struct {
	CabinetPath string
	rootUrl     *url.URL
}

func NewFileStorage(cabinetPath string) (*FileStorage, error) {
	s := &FileStorage{
		CabinetPath: cabinetPath,
	}
	rootUrl, err := url.Parse("file://" + s.CabinetPath + "/")
	if err != nil {
		return nil, err
	}
	s.rootUrl = rootUrl
	return s, nil
}

func (s *FileStorage) DownloaderUrl() *url.URL {
	return s.rootUrl
}

func (s *FileStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {
	if !isHash(hash) {
		return fmt.Errorf("%v is not hash", hash)
	}

	dataDir := filepath.Join(s.CabinetPath, "data")
	dir := filepath.Join(dataDir, hash[0:2])
	file := filepath.Join(dir, hash[2:])

	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	_, err = os.Stat(file)
	if os.IsExist(err) {
		return nil
	}

	err = ioutil.WriteFile(file, body, 0777)
	if err != nil {
		return err
	}

	//if Verbose {
	fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	//}

	return nil
}
