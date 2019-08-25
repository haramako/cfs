package cfs

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/natefinch/atomic"
	"golang.org/x/sync/errgroup"
)

type Downloader struct {
	BaseUrl *url.URL
}

func NewDownloader(baseRawurl string) (*Downloader, error) {
	url, err := url.Parse(baseRawurl)
	if err != nil {
		return nil, err
	}

	downloader := &Downloader{
		BaseUrl: url,
	}

	return downloader, nil
}

func (d *Downloader) LoadBucket(location string) (*Bucket, error) {

	b := &Bucket{
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

func (d *Downloader) Sync(b *Bucket, dir string) error {
	for _, c := range b.Contents {
		if Verbose {
			fmt.Printf("downloading %s\n", c.Path)
		}

		data, err := d.Fetch(c.Hash, c.Attr)
		if err != nil {
			return err
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

func (d *Downloader) FetchAll(b *Bucket) error {
	ch := make(chan Content)
	eg, ctx := errgroup.WithContext(context.TODO())
	ctx, cancel := context.WithCancel(ctx)
	for i := 0; i < 32; i++ {
		eg.Go(func() error {
			for c := range ch {
				if Verbose {
					fmt.Printf("downloading %s\n", c.Path)
				}

				_, err := d.Fetch(c.Hash, c.Attr)
				if err != nil {
					cancel()
					return err
				}
			}
			return nil
		})
	}

	for _, c := range b.Contents {
		ch <- c
	}
	close(ch)

	err := eg.Wait()
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

	cache := filepath.Join(GlobalDataCacheDir(), hash)
	_, err := os.Stat(cache)
	if !os.IsNotExist(err) {
		data, err = ioutil.ReadFile(cache)
		if err != nil {
			return nil, err
		}
	} else {

		fetchUrl, err := d.BaseUrl.Parse(fmt.Sprintf("data/%s/%s", hash[0:2], hash[2:]))
		if err != nil {
			return nil, err
		}

		data, err = fetch(fetchUrl)
		if err != nil {
			return nil, err
		}

		err = atomic.WriteFile(cache, bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
	}

	return decode(data, Option.EncryptKey, Option.EncryptIv, attr)
}

func (d *Downloader) FetchTag(tag string) ([]byte, error) {

	fetchUrl, err := d.BaseUrl.Parse("tag/" + tag)
	if err != nil {
		return nil, err
	}

	data, err := fetch(fetchUrl)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func fetch(_url *url.URL) ([]byte, error) {
	t := &http.Transport{}
	if isWindows() {
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("")))
	} else {
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	}
	c := &http.Client{Transport: t}

	res, err := c.Get(_url.String())
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return contents, nil
}
