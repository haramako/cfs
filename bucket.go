package cfs

import (
	"crypto/md5"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ExcludePatterns = []string{".*", "*.vdat", "cfs", "*.meta", "*.tmx"}
var Verbose = false

type Bucket struct {
	Tag         string
	Path        string
	Hash        string
	ExcludeList []string
	Contents    map[string]Content
	changed     bool
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
	b := &Bucket{
		Path:     path,
		Contents: make(map[string]Content),
		HashType: "md5",
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
