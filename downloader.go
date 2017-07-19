package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type Downloader struct {
	BaseUrl *url.URL
}

func NewDownloader(base_rawurl string) (*Downloader, error) {
	url, err := url.Parse(base_rawurl)
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
		location: location,
		HashType: "md5",
	}

	var body []byte
	if !isHash(location) {
		return nil, fmt.Errorf("%s is not hash", location)
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

		err = ioutil.WriteFile(filepath.Join(dir, filepath.FromSlash(c.Path)), data, 0666)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Downloader) Fetch(hash string, attr ContentAttribute) ([]byte, error) {
	if !isHash(hash) {
		return nil, fmt.Errorf("cannot fetch data, %s is not a hash", hash)
	}

	fetch_url, err := d.BaseUrl.Parse(fmt.Sprintf("data/%s/%s", hash[0:2], hash[2:]))
	if err != nil {
		return nil, err
	}

	data, err := fetch(fetch_url)
	if err != nil {
		return nil, err
	}

	if attr.Crypted() {
		block, err := aes.NewCipher([]byte(Option.EncryptKey))
		if err != nil {
			panic(err)
		}
		cfb := cipher.NewCFBDecrypter(block, []byte(Option.EncryptIv))
		plain_data := make([]byte, len(data))
		cfb.XORKeyStream(plain_data, data)
		data = plain_data
	}

	if attr.Compressed() {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		data, err = ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
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
