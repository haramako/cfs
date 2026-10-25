package cfs

import (
	"errors"
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

func TestUploadFailureKeepsManifest(t *testing.T) {
	storage, _ := NewDummyStorage("")
	storage.onPut = func(key string, body []byte, overwrite bool) error {
		if strings.HasPrefix(key, "data/") && string(body) == "broken" {
			return errors.New("put failed")
		}
		return nil
	}

	manifest := NewManifest()
	manifest.Tag = "v1"
	client := &Client{Manifest: manifest, Storage: storage, Config: &Config{}}
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	client.AddContent("ok", []byte("ok"))
	client.AddContent("broken", []byte("broken"))

	// 中身を置けなかったら、マニフェストもタグも置かずに失敗する
	if err := client.Finish(); err == nil || !strings.Contains(err.Error(), "put failed") {
		t.Fatalf("finish must fail, but %v", err)
	}
	if list, _ := storage.List("manifest/"); len(list) != 0 {
		t.Errorf("manifest must not be uploaded: %v", list)
	}
	if ok, _ := storage.Exists(tagKey("v1")); ok {
		t.Errorf("tag must not be uploaded")
	}
}

func TestClientAttributeFromPatterns(t *testing.T) {
	storage, _ := NewDummyStorage("")
	cfg := DefaultConfig()
	cfg.EncryptKey = "attribute-test-key-1234"
	cfg.NoEncrypt = []string{"*.ab", "Public/", "!secret.ab"}
	cfg.NoCompress = []string{"*.ab", "*.mp4"}
	client := &Client{Manifest: NewManifest(), Storage: storage, Config: cfg}
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	for path, expect := range map[string]ContentAttribute{
		"a.txt":            Compressed | Crypted,
		"bundles/x.ab":     NoContentAttribute,
		"movie.mp4":        Crypted,
		"Public/a.txt":     Compressed,
		"Public/sub/b.png": Compressed,
		"secret.ab":        Crypted, // "!" で暗号化に戻す
	} {
		if got := client.attribute(path); got != expect {
			t.Errorf("attribute(%q) = %v, want %v", path, got, expect)
		}
	}

	// 誤ったパターンは、どの設定かが分かるエラーにする
	bad := &Client{Manifest: NewManifest(), Storage: storage, Config: &Config{NoEncrypt: []string{"[bad"}}}
	if err := bad.Init(); err == nil || !strings.Contains(err.Error(), "NoEncrypt") {
		t.Errorf("invalid NoEncrypt must be an error, but %v", err)
	}
}
