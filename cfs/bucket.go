package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ExcludePetterns = []string{".*", "*.vdat", "cfs", "*.meta"}
var Verbose = false

type Bucket struct {
	Tag         string
	Path        string
	Hash        string
	ExcludeList []string
	Contents    map[string]Content
	uploader    Uploader
	changed     bool
	url         string
	location    string
	HashType    string
}

type Content struct {
	Path     string
	Hash     string
	Time     time.Time
	Size     int
	OrigHash string
	OrigSize int
	Touched  bool
}

type Uploader interface {
	Upload(path string, body []byte, overwrite bool) error
	Close()
	Stat() *UploaderStat
}

type UploaderStat struct {
	UploadCount int
}

func BucketFromFile(path string, uploader Uploader) (*Bucket, error) {
	b := &Bucket{
		Path:     path,
		Contents: make(map[string]Content),
		uploader: uploader,
		HashType: "md5",
	}
	data, err := ioutil.ReadFile(path)
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

func BucketFromUrlOnly(url string) *Bucket {
	b := &Bucket{
		Contents: make(map[string]Content),
		url:      url,
		location: "",
		HashType: "md5",
	}
	return b
}

func BucketFromUrl(url string, location string) (*Bucket, error) {
	b := &Bucket{
		Contents: make(map[string]Content),
		url:      url,
		location: location,
		HashType: "md5",
	}

	var body []byte
	var err error
	if strings.HasPrefix(location, "/data/") {
		body, err = b.Fetch(location[len("/data/"):])
		if err != nil {
			return nil, err
		}
	} else {
		hash, err := fetch(url + location)
		if err != nil {
			return nil, err
		}
		body, err = b.Fetch(string(hash))
		if err != nil {
			return nil, err
		}
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
				b.Contents[col[1]] = Content{Hash: col[0], Path: col[1], Size: size, Time: time}
			}
		}
	}
	return nil
}

func (b *Bucket) Sum(data []byte) string {
	switch b.HashType {
	case "sha1":
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
		r = append(r, strings.Join([]string{
			c.Hash, c.Path, strconv.Itoa(c.Size), c.Time.Format(time.RFC3339),
			c.OrigHash, strconv.Itoa(c.OrigSize),
		}, "\t"))
	}

	return strings.Join(r, "\n") + "\n"
}

func (b *Bucket) Finish() error {
	orig_data := []byte(b.Dump())
	orig_hash := b.Sum(orig_data)

	if b.uploader != nil {
		hash, _, err := b.Upload(orig_data, orig_hash)
		if err != nil {
			return err
		}
		b.Hash = hash

		if b.changed && b.Tag != "" {
			err := b.uploader.Upload("tags/"+b.Tag, []byte(hash), true)
			if err != nil {
				return err
			}

			err = b.uploader.Upload("versions/"+b.Tag+time.Now().Format("-2006-01-02-150405"), []byte(hash), true)
			if err != nil {
				return err
			}
		}
	}

	err := ioutil.WriteFile(b.Path, orig_data, 0666)
	if err != nil {
		return err
	}

	ioutil.WriteFile(b.Path+".hash", []byte(b.Hash), 0666)
	if Verbose {
		fmt.Printf("write bucket to '%s' (%s)\n", b.Path, b.Hash)
	}

	return nil
}

func (b *Bucket) Close() {
	if b.uploader != nil {
		b.uploader.Close()
	}
}

func (b *Bucket) Upload(orig_data []byte, orig_hash string) (string, int, error) {
	data := orig_data
	hash := orig_hash
	if Option.Compress {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(orig_data)
		w.Close()
		data = buf.Bytes()
		hash = b.Sum(data)
	}

	if b.uploader != nil {
		err := b.uploader.Upload("data/"+hash, data, false)
		if err != nil {
			panic(err)
		}
	}

	return hash, len(data), nil
}

func (b *Bucket) AddFile(root string, relative string, info os.FileInfo) bool {
	fullPath := path.Join(root, relative)
	old, found := b.Contents[relative]
	if found {
		b.Contents[relative] = Content{
			Path: old.Path, Hash: old.Hash, Time: old.Time, Size: old.Size,
			OrigHash: old.OrigHash, OrigSize: old.OrigSize, Touched: true,
		}
		if !info.ModTime().After(old.Time) {
			return false
		}
	}

	orig_data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		panic(err)
	}

	orig_hash := b.Sum(orig_data)
	if found && old.OrigHash == orig_hash {
		return false
	}

	if Verbose {
		fmt.Printf("changed file %-12s (%s)\n", relative, orig_hash)
	}

	hash, size, err := b.Upload(orig_data, orig_hash)
	if err != nil {
		panic(err)
	}

	b.Contents[relative] = Content{
		Path: relative, Hash: hash, Size: size,
		OrigHash: orig_hash, OrigSize: len(orig_data), Time: info.ModTime(), Touched: true,
	}
	b.changed = true

	return true
}

func (b *Bucket) AddFiles(root string) {
	filepath.Walk(root, func(path2 string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			if Verbose {
				fmt.Printf("%s not found\n", path2)
			}
			return nil
		}

		// TODO: filepath.Matchがちゃんと働かないため、とりあえず対応
		if filepath.Ext(path2) == ".meta" {
			return nil
		}

		if path2 != "." {
			for _, pat := range ExcludePetterns {
				if match, err := filepath.Match(pat, path2); match || err != nil {
					if info.Mode().IsDir() {
						return filepath.SkipDir
					} else {
						return nil
					}
				}
			}
		}

		if !info.Mode().IsDir() {
			if root == "." || root == path2 {
				b.AddFile("", path2, info)
			} else {
				b.AddFile(root, path2[len(root)+1:], info)
			}
		} else {
			if !Option.Recursive && root != path2 {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

func (b *Bucket) Sync(dir string) error {
	for _, c := range b.Contents {
		data, err := b.Fetch(c.Hash)
		if err != nil {
			return err
		}

		err = os.MkdirAll(path.Dir(path.Join(dir, c.Path)), 0777)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(path.Join(dir, c.Path), data, 0666)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Bucket) Fetch(hash string) ([]byte, error) {
	data, err := fetch(b.url + "/data/" + hash)
	if err != nil {
		return nil, err
	}

	if Option.Compress {
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

func fetch(url string) ([]byte, error) {
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	c := &http.Client{Transport: t}

	res, err := c.Get(url)
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
