package cfs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type FileUploader struct {
	opt  *FileOption
	stat UploaderStat
}

type FileOption struct {
	Root string
}

func CreateFileUploader(option *FileOption) (Uploader, error) {
	return &FileUploader{opt: option}, nil
}

func (u *FileUploader) Upload(_path string, body []byte, overwrite bool) error {
	fullpath := filepath.Join(filepath.FromSlash(u.opt.Root), filepath.FromSlash(_path))
	dir, _ := filepath.Split(filepath.FromSlash(fullpath))

	if !overwrite {
		stat, err := os.Stat(filepath.FromSlash(fullpath))
		if err == nil && stat != nil {
			return nil
		}
	}

	err := os.MkdirAll(filepath.FromSlash(dir), 0777)
	if err != nil {
		return err
	}

	u.stat.UploadCount++
	err = ioutil.WriteFile(filepath.FromSlash(fullpath), body, 0666)
	if err != nil {
		return err
	}

	if Verbose {
		fmt.Printf("uploading '%s'\n", _path)
	}

	return nil
}

func (u *FileUploader) Close() {
}

func (u *FileUploader) Stat() *UploaderStat {
	return &u.stat
}
