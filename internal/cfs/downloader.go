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

type Downloader struct {
	fetcher Fetcher

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

// NewDownloader は、ダウンロード元（URL か手元のパス）からデータを読む Downloader を作る
func NewDownloader(location string) (*Downloader, error) {
	fetcher, err := NewFetcher(location)
	if err != nil {
		return nil, err
	}

	return &Downloader{fetcher: fetcher}, nil
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
// .cfsenv に VerifyKeys があれば、署名を確かめる
func (d *Downloader) LoadManifest(location string) (*Manifest, error) {
	if err := d.checkFormat(); err != nil {
		return nil, err
	}

	hash, err := d.ResolveLocation(location)
	if err != nil {
		return nil, err
	}

	body, err := d.fetchObject(manifestKey(hash), hash, DefaultContentAttribute())
	if err != nil {
		return nil, err
	}

	verifier, err := Option.Verifier()
	if err != nil {
		return nil, err
	}

	m := NewManifest()
	m.Hash = hash
	if err := m.Parse(body, verifier); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", hash, err)
	}

	return m, nil
}

func (d *Downloader) ExistsAll(m *Manifest) (map[string]bool, error) {
	result := map[string]bool{}
	mutex := sync.Mutex{}

	wg := sync.WaitGroup{}
	ch := make(chan Content, 32)

	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range ch {
				if Verbose {
					fmt.Printf("verifying %s (%s)\n", c.Path, c.Hash)
				}
				// 0 バイトのデータは置かれない
				exists := c.Size == 0
				if !exists {
					// 確認できなかった場合も、ないものとして扱う
					exists, _ = d.fetcher.Exists(dataKey(c.Hash))
				}
				mutex.Lock()
				result[c.Path] = exists
				mutex.Unlock()
			}
		}()
	}

	for _, c := range m.Contents {
		ch <- c
	}

	close(ch)
	wg.Wait()

	return result, nil
}

func (d *Downloader) Sync(m *Manifest, dir string) error {
	for _, c := range m.Contents {
		if Verbose {
			fmt.Printf("downloading %s\n", c.Path)
		}

		// 0 バイトのデータは置かれないので、空ファイルを作る
		var err error
		data := []byte{}
		if c.Size > 0 {
			data, err = d.Fetch(c.Hash, c.Attr)
			if err != nil {
				return err
			}
		}

		err = os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(c.Path))), 0777)
		if err != nil {
			return err
		}

		err = atomic.WriteFile(filepath.Join(dir, filepath.FromSlash(c.Path)), bytes.NewBuffer(data))
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Downloader) FetchAll(m *Manifest) error {
	const RETRY_LIMIT = 3
	limit := make(chan struct{}, 8)
	eg, ctx := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	for _, c := range m.Contents {
		c := c
		eg.Go(func() error {
			limit <- struct{}{}
			defer func() { <-limit }()

			select {
			case <-ctx.Done():
				return nil
			default:
				if Verbose {
					fmt.Printf("downloading %s\n", c.Path)
				}

				// 0 バイトのデータは置かれないので、取得しない
				if c.Size == 0 {
					return nil
				}

				retryCount := 0
				for {
					_, err := d.Fetch(c.Hash, c.Attr)
					if err != nil {
						if retryCount < RETRY_LIMIT {
							retryCount++
							fmt.Printf("retry for %v, retry count %d\n", err, retryCount)
							continue
						} else {
							return err
						}
					} else {
						break
					}
				}

			}
			return nil
		})
	}

	err := eg.Wait()
	cancel()
	if err != nil {
		return err
	}

	return nil
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
	return d.fetchObject(dataKey(hash), hash, attr)
}

// fetchObject は、キャビネットの key のデータを（手元のキャッシュを使って）取得し、ハッシュを確かめて元に戻す
func (d *Downloader) fetchObject(key string, hash string, attr ContentAttribute) ([]byte, error) {
	cache := filepath.Join(GlobalDataCacheDir(), hash)

	data, err := os.ReadFile(cache)
	if err != nil || SumHash(data) != hash {
		// キャッシュがない、または壊れているので取得し直す
		data, err = fetchAll(d.fetcher, key)
		if err != nil {
			return nil, err
		}
		if SumHash(data) != hash {
			return nil, fmt.Errorf("hash mismatch for %s (got %s)", key, SumHash(data))
		}
		err = atomic.WriteFile(cache, bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
	}

	keys, err := Option.KeyRing()
	if err != nil {
		return nil, err
	}
	return decode(data, keys, attr)
}

func (d *Downloader) FetchTag(tag string) ([]byte, error) {
	return fetchAll(d.fetcher, tagKey(tag))
}
