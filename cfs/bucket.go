package cfs

import (
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

var ExcludePetterns = []string{".*", "*.vdat", "cfs"}
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
}

type Content struct {
	Path    string
	Hash    string
	Time    time.Time
	Size    int
	Touched bool
}

type Uploader interface {
	Upload(path string, body []byte, overwrite bool) error
	Close()
}

func BucketFromFile(path string, uploader Uploader) (*Bucket, error) {
	b := &Bucket{
		Path:     path,
		Contents: make(map[string]Content),
		uploader: uploader,
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

func BucketFromUrl(url string, location string) (*Bucket, error) {
	b := &Bucket{
		Contents: make(map[string]Content),
		url:      url,
		location: location,
	}

	body, err := fetch(path.Join(url, location))
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
				b.Contents[col[1]] = Content{Hash: col[0], Path: col[1], Size: size, Time: time}
			}
		}
	}
	return nil
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
		r = append(r, strings.Join([]string{c.Hash, c.Path, strconv.Itoa(c.Size), c.Time.Format(time.RFC3339)}, "\t"))
	}

	return strings.Join(r, "\n") + "\n"
}

func (b *Bucket) Finish() error {
	dump := []byte(b.Dump())
	b.Hash = fmt.Sprintf("%x", sha1.Sum(dump))

	if b.uploader != nil {
		err := b.uploader.Upload("data/"+b.Hash, dump, false)
		if err != nil {
			return err
		}

		if b.changed && b.Tag != "" {
			err = b.uploader.Upload("tags/"+b.Tag, dump, true)
			if err != nil {
				return err
			}

			err = b.uploader.Upload("versions/"+b.Tag+time.Now().Format("-2006-01-02-150405"), dump, true)
			if err != nil {
				return err
			}
		}
	}

	err := ioutil.WriteFile(b.Path, dump, 0666)
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

func (b *Bucket) AddFile(path string, info os.FileInfo) bool {
	old, found := b.Contents[path]
	if found {
		b.Contents[path] = Content{Path: old.Path, Hash: old.Hash, Time: old.Time, Size: old.Size, Touched: true}
		if !info.ModTime().After(old.Time) {
			return false
		}
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	hash := fmt.Sprintf("%x", sha1.Sum(data))
	if found && old.Hash == hash {
		return false
	}

	if Verbose {
		fmt.Printf("changed file %-12s (%s)\n", path, hash)
	}
	if b.uploader != nil {
		b.uploader.Upload("data/"+hash, data, false)
	}

	b.Contents[path] = Content{Path: path, Hash: hash, Time: info.ModTime(), Size: len(data), Touched: true}
	b.changed = true

	return true
}

func (b *Bucket) AddFiles(path string) {
	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			if Verbose {
				fmt.Printf("%s not found\n", path)
			}
			return nil
		}

		if path != "." {
			for _, pat := range ExcludePetterns {
				if match, err := filepath.Match(pat, path); match || err != nil {
					if info.Mode().IsDir() {
						return filepath.SkipDir
					} else {
						return nil
					}
				}
			}
		}

		if !info.Mode().IsDir() {
			b.AddFile(path, info)
		}
		return nil
	})
}

func (b *Bucket) Sync(dir string) error {
	for _, c := range b.Contents {
		data, err := fetch(path.Join(b.url, "data", c.Hash))
		if err != nil {
			return err
		}

		err = os.MkdirAll(path.Join(dir), 0777)
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
