package cfs

import (
	"testing"

	"github.com/pkg/errors"
)

func TestUploadWithFailure(t *testing.T) {
	bucket := &Bucket{HashType: "md5", Contents: make(map[string]Content)}

	storage, err := NewDummyStorage("")
	if err != nil {
		t.Errorf("Can't create DummyStorage.")
	}
	storage.onUpload = func(filename string, hash string, body []byte, overwrite bool) error {
		if filename == "error" {
			return errors.Errorf("error for DummyStorage")
		}
		return nil
	}

	client := &Client{
		Bucket:  bucket,
		Storage: storage,
	}
	client.Init()

	client.AddContent("hoge", []byte("hoge"))
	client.AddContent("fuga", []byte("hoge"))
	client.AddContent("error", []byte("error"))

	err = client.Finish()
	if err != nil {
		t.Error(err)
	}

}
