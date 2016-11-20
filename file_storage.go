package cfs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type FileStorage struct {
	CabinetPath string
}

func (s *FileStorage) Init() error {
	return nil
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
