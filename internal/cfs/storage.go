package cfs

import (
	"fmt"
	"net/url"
)

type Storage interface {
	DownloaderUrl() *url.URL
	Upload(filename string, hash string, body []byte, overwrite bool) error
	UploadTag(filename string, body []byte) error
}

func StorageFromUrl(cabinetUrl *url.URL) (Storage, error) {
	// ストレージを足すときは、ここに URL スキームを追加する
	switch cabinetUrl.Scheme {
	case "gs":
		return nil, fmt.Errorf("gs:// (Google Cloud Storage) is not supported yet: %v", cabinetUrl)
	case "s3":
		storage, err := NewS3Storage(cabinetUrl.Host)
		if err != nil {
			return nil, err
		}
		return storage, nil
	case "file":
		path, err := FileURLToPath(cabinetUrl)
		if err != nil {
			return nil, err
		}
		return NewFileStorage(path)
	default:
		return nil, fmt.Errorf("invalid url %v", cabinetUrl)
	}
}

// StorageFromString は、キャビネットの指定（URL か手元のパス）からストレージを作る
func StorageFromString(cabinet string) (Storage, error) {
	if !IsURL(cabinet) {
		return NewFileStorage(cabinet)
	}
	cabinetUrl, err := url.Parse(cabinet)
	if err != nil {
		return nil, err
	}
	return StorageFromUrl(cabinetUrl)
}
