package cfs

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type FileStorage struct {
	CabinetPath string
	rootUrl     *url.URL
}

func NewFileStorage(cabinetPath string) (*FileStorage, error) {
	s := &FileStorage{
		CabinetPath: cabinetPath,
	}

	if isWindows() {
		cabinetPath = strings.Replace(cabinetPath, "\\", "/", -1)
	}

	rootUrl, err := url.Parse("file://" + cabinetPath + "/")
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

	cabinetPath := s.CabinetPath
	if isWindows() {
		if cabinetPath[0] == '/' {
			// Windowsの場合 "file:///C:/dir/path" の場合に
			// cabinetPath が "/C:/dir/path" ではなく "C:/dir/path" になるように調整する
			cabinetPath = cabinetPath[1:]
		}
	}

	dataDir := filepath.Join(cabinetPath, "data")
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

func (s *FileStorage) UploadTag(filename string, body []byte) error {
	cabinetFilepath := s.CabinetPath
	if isWindows() {
		if cabinetFilepath[0] == '/' {
			cabinetFilepath = cabinetFilepath[1:]
		}
	}

	dataDir := filepath.Join(cabinetFilepath, "tag")
	file := filepath.Join(dataDir, filename)

	err := os.MkdirAll(dataDir, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(file, body, 0777)
	if err != nil {
		return err
	}

	//if Verbose {
	fmt.Printf("uploading tag '%s'\n", filename)
	//}

	return nil
}
