package cfs

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeGcsInsert struct {
	Name              string
	IfGenerationMatch string
	Body              string
}

// fakeGcs は JSON API のオブジェクト取得とマルチパートアップロードだけを受け付ける GCS の偽物
type fakeGcs struct {
	mutex   sync.Mutex
	objects map[string]bool
	inserts []fakeGcsInsert
}

func (f *fakeGcs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/storage/v1/b/bkt/o/"):
		name := strings.TrimPrefix(r.URL.Path, "/storage/v1/b/bkt/o/")
		if !f.objects[name] {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":{"code":404,"message":"No such object"}}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"bucket": "bkt", "name": name})
	case r.Method == http.MethodPost && r.URL.Path == "/upload/storage/v1/b/bkt/o":
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		var meta struct{ Name string }
		metaPart, _ := mr.NextPart()
		json.NewDecoder(metaPart).Decode(&meta)
		mediaPart, _ := mr.NextPart()
		body, _ := io.ReadAll(mediaPart)

		f.objects[meta.Name] = true
		f.inserts = append(f.inserts, fakeGcsInsert{
			Name:              meta.Name,
			IfGenerationMatch: r.URL.Query().Get("ifGenerationMatch"),
			Body:              string(body),
		})
		json.NewEncoder(w).Encode(map[string]string{"bucket": "bkt", "name": meta.Name})
	default:
		http.Error(w, "unexpected request "+r.Method+" "+r.URL.String(), http.StatusBadRequest)
	}
}

func setupFakeGcs(t *testing.T) (*fakeGcs, *GcsStorage) {
	fake := &fakeGcs{objects: map[string]bool{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	// エミュレーター指定にすると認証なしで任意のエンドポイントへ接続する
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL)

	s, err := NewGcsStorage("bkt")
	if err != nil {
		t.Fatal(err)
	}
	return fake, s
}

func TestGcsUpload(t *testing.T) {
	fake, s := setupFakeGcs(t)
	hash := "0123456789abcdef0123456789abcdef"

	if err := s.Upload("hoge", hash, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.inserts) != 1 {
		t.Fatalf("expect 1 insert but %d", len(fake.inserts))
	}
	ins := fake.inserts[0]
	if ins.Name != "data/01/23456789abcdef0123456789abcdef" || ins.Body != "hoge" || ins.IfGenerationMatch != "0" {
		t.Errorf("invalid insert %+v", ins)
	}

	// 既に存在するなら送信しない
	if err := s.Upload("hoge", hash, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.inserts) != 1 {
		t.Errorf("uploaded existing object")
	}
}

func TestGcsUploadTag(t *testing.T) {
	fake, s := setupFakeGcs(t)

	if err := s.UploadTag("v1", []byte("0123456789abcdef0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	if len(fake.inserts) != 1 {
		t.Fatalf("expect 1 insert but %d", len(fake.inserts))
	}
	ins := fake.inserts[0]
	if ins.Name != "tag/v1" || ins.Body != "0123456789abcdef0123456789abcdef" || ins.IfGenerationMatch != "" {
		t.Errorf("invalid insert %+v", ins)
	}
}

func TestGcsDownloaderUrl(t *testing.T) {
	_, s := setupFakeGcs(t)

	if s.DownloaderUrl().String() != "http://storage.googleapis.com/bkt/" {
		t.Errorf("invalid url %v", s.DownloaderUrl())
	}
}
