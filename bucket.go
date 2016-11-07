package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ExcludePatterns = []string{".*", "*.vdat", "cfs", "*.meta", "*.tmx"}
var Verbose = false

type Storage interface {
	Init() error
	Upload(filename string, hash string, body []byte, overwrite bool) error
}

type Bucket struct {
	Tag         string
	Path        string
	Hash        string
	ExcludeList []string
	Contents    map[string]Content
	CabinetUrl  *url.URL
	changed     bool
	BaseUrl     *url.URL
	location    string
	HashType    string
	UploadCount int
}

const (
	Compressed = 1
	Crypted    = 2
)

type ContentAttribute int

type Content struct {
	Path     string
	Hash     string
	Time     time.Time
	Size     int
	OrigHash string
	OrigSize int
	Attr     ContentAttribute
	Touched  bool
}

func DefaultContentAttribute() ContentAttribute {
	var result = 0
	if Option.Compress {
		result |= Compressed
	}
	if Option.EncryptKey != "" {
		result |= Crypted
	}
	return ContentAttribute(result)
}

func (c ContentAttribute) Compressed() bool {
	return (int(c) & Compressed) != 0
}

func (c ContentAttribute) Crypted() bool {
	return (int(c) & Crypted) != 0
}

func BucketFromFile(path string) (*Bucket, error) {
	cabinetUrl, err := url.Parse(Option.Cabinet)
	if err != nil {
		return nil, fmt.Errorf("invalid cabinet url '%s'", Option.Cabinet)
	}

	b := &Bucket{
		Path:       path,
		Contents:   make(map[string]Content),
		CabinetUrl: cabinetUrl,
		HashType:   "md5",
	}
	data, err := ioutil.ReadFile(filepath.FromSlash(path))
	if err == nil {
		if Verbose {
			fmt.Printf("read bucket from '%s'\n", b.Path)
		}
		err := b.Parse(data)
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

func BucketFromUrl(base_rawurl string, location string) (*Bucket, error) {
	base_url, err := url.Parse(base_rawurl)
	if err != nil {
		return nil, err
	}

	b := &Bucket{
		Contents: make(map[string]Content),
		BaseUrl:  base_url,
		location: location,
		HashType: "md5",
	}

	var body []byte
	if !isHash(location) {
		return nil, fmt.Errorf("%s is not hash", location)
	}
	body, err = b.Fetch(location, DefaultContentAttribute())
	if err != nil {
		return nil, err
	}

	err = b.Parse(body)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bucket) Parse(s []byte) error {
	for _, line := range strings.Split(string(s), "\n") {
		if len(s) != 0 {
			col := strings.Split(line, "\t")
			if len(col) >= 3 {
				size, err := strconv.Atoi(col[2])
				if err != nil {
					return err
				}
				time, err := time.Parse(time.RFC3339, col[3])
				if err != nil {
					return err
				}
				origSize, err := strconv.Atoi(col[5])
				if err != nil {
					return err
				}
				attr, err := strconv.Atoi(col[6])
				if err != nil {
					return err
				}
				b.Contents[col[1]] = Content{
					Hash:     col[0],
					Path:     col[1],
					Size:     size,
					Time:     time,
					OrigHash: col[4],
					OrigSize: origSize,
					Attr:     ContentAttribute(attr),
				}
			}
		}
	}
	return nil
}

func (b *Bucket) Sum(data []byte) string {
	switch b.HashType {
	case "sha1":
		panic("not supported")
		return fmt.Sprintf("%x", sha1.Sum(data))
	case "md5":
		return fmt.Sprintf("%x", md5.Sum(data))
	default:
		panic("invalid hash type")
	}
}

func (b *Bucket) RemoveUntouched() {
	newContents := make(map[string]Content)
	for _, c := range b.Contents {
		if c.Touched {
			newContents[c.Path] = c
		} else {
			if Verbose {
				fmt.Printf("removed: %s\n", c.Path)
			}
		}
	}
	b.Contents = newContents
}

func (b *Bucket) Dump() string {
	keys := []string{}
	for k := range b.Contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	r := make([]string, 0)
	for _, k := range keys {
		c := b.Contents[k]
		r = append(
			r,
			strings.Join(
				[]string{
					c.Hash,
					c.Path,
					strconv.Itoa(c.Size),
					c.Time.Format(time.RFC3339),
					c.OrigHash,
					strconv.Itoa(c.OrigSize),
					strconv.Itoa(int(c.Attr)),
				},
				"\t"))
	}

	return strings.Join(r, "\n") + "\n"
}
func (b *Bucket) GetAttribute(path string) ContentAttribute {
	var attr = DefaultContentAttribute()
	// TODO: とりあえずフィルタを固定している
	if filepath.Ext(path) == ".ab" || filepath.Ext(path) == ".raw" || filepath.Ext(path) == ".pbx" || filepath.Ext(path) == ".mp4" {
		attr = ContentAttribute(0)
	}
	return attr
}

func (b *Bucket) Sync(dir string) error {
	for _, c := range b.Contents {
		if Verbose {
			fmt.Printf("downloading %s\n", c.Path)
		}

		data, err := b.Fetch(c.Hash, c.Attr)
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

func (b *Bucket) Fetch(hash string, attr ContentAttribute) ([]byte, error) {
	if !isHash(hash) {
		return nil, fmt.Errorf("cannot fetch data, %s is not a hash", hash)
	}

	fetch_url, err := b.BaseUrl.Parse(fmt.Sprintf("data/%s/%s", hash[0:2], hash[2:]))
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

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}
