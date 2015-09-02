package cfs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
)

type FileUploader struct {
	Root string
}

func CreateFileUploader(root string) (Uploader, error) {
	return &FileUploader{Root: root}, nil
}

func (u *FileUploader) Upload(_path string, body []byte, overwrite bool) error {
	fullpath := path.Join(u.Root, _path)
	dir, _ := path.Split(fullpath)

	if !overwrite {
		stat, err := os.Stat(fullpath)
		if err == nil && stat != nil {
			return nil
		}
	}

	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fullpath, body, 0666)
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
