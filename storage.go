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
	switch cabinetUrl.Scheme {
	case "http", "https":
		return NewCfsStorage(cabinetUrl)
	case "cfs":
		httpUrl := *cabinetUrl
		httpUrl.Scheme = "http"
		return NewCfsStorage(&httpUrl)
	case "gs":
		storage, err := NewGcsStorage(cabinetUrl.Host)
		if err != nil {
			return nil, err
		}
		return storage, nil
	case "s3":
		storage, err := NewS3Storage(cabinetUrl.Host)
		if err != nil {
			return nil, err
		}
		return storage, nil
	case "file":
		return NewFileStorage(cabinetUrl.Path)
	default:
		return nil, fmt.Errorf("invalid url %v", cabinetUrl)
	}
}

func StorageFromString(cabinetUrlStr string) (Storage, error) {
	cabinetUrl, err := url.Parse(cabinetUrlStr)
	if err != nil {
		return nil, err
	}
	return StorageFromUrl(cabinetUrl)
}
