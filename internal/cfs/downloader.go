package cfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/natefinch/atomic"
	"golang.org/x/sync/errgroup"
)

type Downloader struct {
	fetcher Fetcher
}

// NewDownloader は、ダウンロード元（URL か手元のパス）からデータを読む Downloader を作る
func NewDownloader(location string) (*Downloader, error) {
	fetcher, err := NewFetcher(location)
	if err != nil {
		return nil, err
	}

	return &Downloader{fetcher: fetcher}, nil
}

func (d *Downloader) LoadManifest(location string) (*Manifest, error) {

	b := &Manifest{
		Contents: make(map[string]Content),
		HashType: "md5",
	}

	var body []byte
	if !isHash(location) {
		locationBytes, err := d.FetchTag(location)
		if err != nil {
			return nil, err
		}
		location = string(locationBytes)
		if !isHash(location) {
			return nil, fmt.Errorf("%s is not hash", location)
		}
	}

	body, err := d.Fetch(location, DefaultContentAttribute())
	if err != nil {
		return nil, err
	}

	err = b.Parse(body)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (d *Downloader) ExistsAll(b *Manifest) (map[string]bool, error) {
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
				// 確認できなかった場合も、ないものとして扱う
				exists, _ := d.fetcher.Exists(dataPath(c.Hash))
				mutex.Lock()
				result[c.Path] = exists
				mutex.Unlock()
			}
		}()
	}

	for _, c := range b.Contents {
		ch <- c
	}

	close(ch)
	wg.Wait()

	return result, nil
}

func (d *Downloader) Sync(b *Manifest, dir string) error {
	for _, c := range b.Contents {
		if Verbose {
			fmt.Printf("downloading %s\n", c.Path)
		}

		// TODO: 0 bytesのファイルはアップロードがされていないため、空ファイルを作る
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

func (d *Downloader) FetchAll(b *Manifest) error {
	const RETRY_LIMIT = 3
	limit := make(chan struct{}, 8)
	eg, ctx := errgroup.WithContext(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	for _, c := range b.Contents {
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

				// TODO: 0 bytesのファイルはアップロードがされていないため、空ファイルを作る
				if c.Size == 0 {
					os.Create(filepath.Join(GlobalDataCacheDir(), c.Hash))
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

func (d *Downloader) Fetch(hash string, attr ContentAttribute) ([]byte, error) {
	if !isHash(hash) {
		return nil, fmt.Errorf("cannot fetch data, %s is not a hash", hash)
	}

	var data []byte

	// データをキャッシュしているパス取得
	cache := filepath.Join(GlobalDataCacheDir(), hash)
	_, err := os.Stat(cache)
	if !os.IsNotExist(err) {
		data, err = os.ReadFile(cache)
		if err != nil {
			return nil, err
		}
	} else {

		// ファイルダウンロード
		data, err = fetchAll(d.fetcher, dataPath(hash))
		if err != nil {
			return nil, err
		}

		// データファイルをキャッシュする
		err = atomic.WriteFile(cache, bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
	}

	return decode(data, Option.EncryptKey, Option.EncryptIv, attr)
}

func (d *Downloader) FetchTag(tag string) ([]byte, error) {
	return fetchAll(d.fetcher, "tag/"+tag)
}

// dataPath は、中身のハッシュからキャビネット上のパスを返す
func dataPath(hash string) string {
	return "data/" + hashPath(hash)
}
