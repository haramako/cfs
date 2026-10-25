package cfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/natefinch/atomic"
	"golang.org/x/sync/errgroup"
)

const (
	downloadWorkers = 8  // 並列にダウンロードする数
	existsWorkers   = 32 // 並列に存在を確かめる数
	fetchRetries    = 3  // ダウンロードに失敗したときにやり直す回数
)

type Downloader struct {
	fetcher  Fetcher
	keys     *KeyRing
	verifier *Verifier
	cacheDir string // ダウンロードしたデータの手元のキャッシュ
	log      Logger

	formatOnce sync.Once
	formatErr  error
}

// checkFormat は、最初の 1 回だけキャビネットの形式の版を確かめる
func (d *Downloader) checkFormat() error {
	d.formatOnce.Do(func() {
		d.formatErr = CheckCabinetFormat(d.fetcher)
	})
	return d.formatErr
}

// NewDownloader は、ダウンロード元（URL か手元のパス）からデータを読む Downloader を作る。cfg が nil なら既定の設定を使う
func NewDownloader(location string, cfg *Config) (*Downloader, error) {
	fetcher, err := OpenFetcher(location, cfg)
	if err != nil {
		return nil, err
	}
	return newDownloader(fetcher, cfg)
}

func newDownloader(fetcher Fetcher, cfg *Config) (*Downloader, error) {
	cfg = cfg.orDefault()
	keys, err := cfg.KeyRing()
	if err != nil {
		return nil, err
	}
	verifier, err := cfg.Verifier()
	if err != nil {
		return nil, err
	}
	cacheDir, err := cfg.DataCacheDir()
	if err != nil {
		return nil, err
	}
	return &Downloader{fetcher: fetcher, keys: keys, verifier: verifier, cacheDir: cacheDir, log: cfg.Logger()}, nil
}

// ResolveLocation は、ロケーション（タグ名かマニフェストのハッシュ）をマニフェストのハッシュにする
func (d *Downloader) ResolveLocation(location string) (string, error) {
	if isHash(location) {
		return location, nil
	}
	data, err := d.FetchTag(location)
	if err != nil {
		return "", err
	}
	hash := strings.TrimSpace(string(data))
	if !isHash(hash) {
		return "", fmt.Errorf("tag %s does not point to a manifest hash: %q", location, hash)
	}
	return hash, nil
}

// LoadManifest は、ロケーション（タグ名かマニフェストのハッシュ）のマニフェストを読む。
// VerifyKeys が設定されていれば、署名を確かめる
func (d *Downloader) LoadManifest(location string) (*Manifest, error) {
	if err := d.checkFormat(); err != nil {
		return nil, err
	}

	hash, err := d.ResolveLocation(location)
	if err != nil {
		return nil, err
	}

	data, err := d.fetchVerified(manifestKey(hash), hash)
	if err != nil {
		return nil, err
	}
	// 上げた側の圧縮・暗号化の設定によらず読めるよう、データから判断する
	body, err := decodeManifest(data, d.keys)
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", hash, err)
	}

	m := NewManifest()
	m.Hash = hash
	if err := m.Parse(body, d.verifier); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", hash, err)
	}

	return m, nil
}

// forEachContent は、マニフェストのファイルごとに f を workers 個まで並列に呼ぶ。最初のエラーで止める
func forEachContent(m *Manifest, workers int, f func(c Content) error) error {
	eg, ctx := errgroup.WithContext(context.Background())
	eg.SetLimit(workers)
	for _, c := range m.Contents {
		eg.Go(func() error {
			if ctx.Err() != nil {
				return nil // ほかで失敗したので、残りはやらない
			}
			return f(c)
		})
	}
	return eg.Wait()
}

// ExistsAll は、マニフェストのファイルのデータがキャビネットにあるかを、パスごとに返す
func (d *Downloader) ExistsAll(m *Manifest) (map[string]bool, error) {
	var mutex sync.Mutex
	result := map[string]bool{}
	err := forEachContent(m, existsWorkers, func(c Content) error {
		d.log.Printf("verifying %s (%s)\n", c.Path, c.Hash)
		// 0 バイトのデータは置かれない
		exists := c.Size == 0
		if !exists {
			// 確認できなかった場合も、ないものとして扱う
			exists, _ = d.fetcher.Exists(dataKey(c.Hash))
		}
		mutex.Lock()
		result[c.Path] = exists
		mutex.Unlock()
		return nil
	})
	return result, err
}

// Sync は、マニフェストのファイルを dir の下に書き出す
func (d *Downloader) Sync(m *Manifest, dir string) error {
	return forEachContent(m, downloadWorkers, func(c Content) error {
		d.log.Printf("downloading %s\n", c.Path)
		data, err := d.fetchContent(c)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, filepath.FromSlash(c.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return err
		}
		return atomic.WriteFile(path, bytes.NewReader(data))
	})
}

// FetchAll は、マニフェストのファイルのデータを、手元のキャッシュに取得する
func (d *Downloader) FetchAll(m *Manifest) error {
	return forEachContent(m, downloadWorkers, func(c Content) error {
		d.log.Printf("downloading %s\n", c.Path)
		_, err := d.fetchContent(c)
		return err
	})
}

// fetchContent は、ファイルのデータを（失敗したらやり直して）取得する
func (d *Downloader) fetchContent(c Content) ([]byte, error) {
	var err error
	for retry := 0; retry <= fetchRetries; retry++ {
		if retry > 0 {
			d.log.Printf("retry %d for %s: %v\n", retry, c.Path, err)
		}
		var data []byte
		if data, err = d.Fetch(c.Hash, c.Attr); err == nil {
			return data, nil
		}
	}
	return nil, err
}

// Fetch は、中身のハッシュのデータを取得して元に戻す
func (d *Downloader) Fetch(hash string, attr ContentAttribute) ([]byte, error) {
	if !isHash(hash) {
		return nil, fmt.Errorf("cannot fetch data, %s is not a hash", hash)
	}
	if hash == SumHash(nil) {
		// 0 バイトのデータは置かれない
		return decode(nil, nil, attr)
	}
	data, err := d.fetchVerified(dataKey(hash), hash)
	if err != nil {
		return nil, err
	}
	return decode(data, d.keys, attr)
}

// fetchVerified は、キャビネットの key のデータを（手元のキャッシュを使って）取得し、ハッシュを確かめる
func (d *Downloader) fetchVerified(key string, hash string) ([]byte, error) {
	cache := filepath.Join(d.cacheDir, hash)
	if data, err := os.ReadFile(cache); err == nil && SumHash(data) == hash {
		return data, nil
	}

	// キャッシュがない、または壊れているので取得し直す
	data, err := fetchAll(d.fetcher, key)
	if err != nil {
		return nil, err
	}
	if SumHash(data) != hash {
		return nil, fmt.Errorf("hash mismatch for %s (got %s)", key, SumHash(data))
	}
	// 同じ中身を同時に書くと置き換えに失敗することがある（Windows）が、キャッシュなので失敗しても続ける
	if err := atomic.WriteFile(cache, bytes.NewReader(data)); err != nil {
		d.log.Printf("cannot write cache %s: %v\n", cache, err)
	}
	return data, nil
}

func (d *Downloader) FetchTag(tag string) ([]byte, error) {
	return fetchAll(d.fetcher, tagKey(tag))
}
