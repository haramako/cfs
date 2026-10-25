package cfs

import (
	"strings"
	"testing"
	"time"
)

func TestUploadWithFailure(t *testing.T) {
	manifest := NewManifest()

	storage, err := NewDummyStorage("")
	if err != nil {
		t.Errorf("Can't create DummyStorage.")
	}
	storage.onPut = func(key string, body []byte, overwrite bool) error {
		if strings.HasPrefix(key, "data/") {
			time.Sleep(time.Millisecond * 10) // Wait a while
		}
		return nil
	}

	client := &Client{
		Manifest: manifest,
		Storage:  storage,
	}
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}

	client.AddContent("hoge", []byte("hoge"))
	client.AddContent("fuga", []byte("hoge"))
	client.AddContent("error", []byte("error"))

	err = client.Finish()
	if err != nil {
		t.Error(err)
	}

	// マニフェストと中身が置かれていること
	if ok, _ := storage.Exists(manifestKey(manifest.Hash)); !ok {
		t.Errorf("manifest must be uploaded")
	}
	for _, c := range manifest.Contents {
		if ok, _ := storage.Exists(dataKey(c.Hash)); !ok {
			t.Errorf("data of %s must be uploaded", c.Path)
		}
	}
}
