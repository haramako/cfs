package cfs

import (
	"fmt"
	"net/url"
	"time"
)

// キャビネットの中の配置（形式 v2）
//
//	data/xx/yyyy...      中身（ハッシュの先頭2文字/残り）。中身は変わらない
//	manifest/xx/yyyy...  マニフェスト
//	tag/<名前>           タグ（中身はマニフェストのハッシュ）
//	config.json          キャビネットの設定（GC の保持期間など）
//	gc/                  GC の状態（消す候補）と記録

// ObjectInfo は、キャビネットに置いたものの情報
type ObjectInfo struct {
	Key          string // "data/xx/yyyy..." のようなキャビネット内の場所
	Size         int64
	LastModified time.Time
}

// Storage は、キャビネットへの書き込み口。ストレージを足すときは、これを実装して StorageFromUrl に追加する
type Storage interface {
	// Put は key にデータを書く。overwrite が false で、既にあれば書かない
	Put(key string, body []byte, overwrite bool) error
	// Exists は key にデータがあるかを返す
	Exists(key string) (bool, error)
	// List は、prefix で始まる key のものを全部返す
	List(prefix string) ([]ObjectInfo, error)
	// Delete は key のものを消す（ないものは無視する）
	Delete(keys []string) error
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
