package cfs

import (
	"errors"
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

// Storage は、キャビネットへの書き込み口。ストレージを足すときは、これを実装して OpenStorage に追加する
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

// OpenStorage は、キャビネットの指定（URL か手元のパス）からストレージを作る。cfg が nil なら既定の設定を使う
func OpenStorage(cabinet string, cfg *Config) (Storage, error) {
	cfg = cfg.orDefault()
	if cabinet == "" {
		return nil, errNoCabinet
	}
	if !IsURL(cabinet) {
		return newFileStorage(cabinet, cfg.Logger())
	}
	cabinetUrl, err := url.Parse(cabinet)
	if err != nil {
		return nil, err
	}
	// ストレージを足すときは、ここに URL スキームを追加する
	switch cabinetUrl.Scheme {
	case "gs":
		return nil, fmt.Errorf("gs:// (Google Cloud Storage) is not supported yet: %v", cabinetUrl)
	case "s3":
		return NewS3Storage(cabinetUrl.Host, cfg.S3Region, cfg.Logger())
	case "file":
		path, err := FileURLToPath(cabinetUrl)
		if err != nil {
			return nil, err
		}
		return newFileStorage(path, cfg.Logger())
	default:
		return nil, fmt.Errorf("invalid url %v", cabinetUrl)
	}
}

var errNoCabinet = errors.New("no cabinet: set Cabinet in .cfsenv or use -c")
