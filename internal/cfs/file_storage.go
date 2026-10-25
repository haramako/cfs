package cfs

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileStorage は、手元のフォルダをキャビネットにする
type FileStorage struct {
	CabinetPath string // キャビネットのフォルダ（絶対パス）
}

// NewFileStorage は、手元のパスのフォルダをキャビネットにする
func NewFileStorage(cabinetPath string) (*FileStorage, error) {
	abs, err := filepath.Abs(cabinetPath)
	if err != nil {
		return nil, err
	}
	return &FileStorage{CabinetPath: abs}, nil
}

func (s *FileStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {
	if !isHash(hash) {
		return fmt.Errorf("%v is not hash", hash)
	}

	dir := filepath.Join(s.CabinetPath, "data", hash[0:2])
	file := filepath.Join(dir, hash[2:])

	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	// 同じ中身は同じ場所になるので、既にあれば書かない
	if !overwrite {
		if _, err := os.Stat(file); err == nil {
			return nil
		}
	}

	err = os.WriteFile(file, body, 0666)
	if err != nil {
		return err
	}

	if Verbose {
		fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	}

	return nil
}

func (s *FileStorage) UploadTag(filename string, body []byte) error {
	dataDir := filepath.Join(s.CabinetPath, "tag")
	file := filepath.Join(dataDir, filename)

	err := os.MkdirAll(dataDir, 0777)
	if err != nil {
		return err
	}

	err = os.WriteFile(file, body, 0666)
	if err != nil {
		return err
	}

	if Verbose {
		fmt.Printf("uploading tag '%s'\n", filename)
	}

	return nil
}
