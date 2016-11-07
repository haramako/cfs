package cfs

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type uploadRequest struct {
	Filename string
	Hash     string
	Data     []byte
}

type Client struct {
	Bucket    *Bucket
	Storage   Storage
	MaxWorker int
	waitGroup sync.WaitGroup
	queue     chan uploadRequest
}

func (c *Client) Init() error {
	if c.MaxWorker == 0 {
		c.MaxWorker = 32
	}

	err := c.Storage.Init()
	if err != nil {
		return err
	}

	c.queue = make(chan uploadRequest, c.MaxWorker)

	c.waitGroup.Add(c.MaxWorker)
	for i := 0; i < c.MaxWorker; i++ {
		go c.uploadWorker()
	}

	return nil
}

func (c *Client) uploadWorker() {
	defer c.waitGroup.Done()

	for req := range c.queue {
		err := c.Storage.Upload(req.Filename, req.Hash, req.Data, false)
		if err != nil {
			fmt.Println(err)
			//return "", 0, err
		}
	}
}

func (c *Client) Upload(filename string, orig_data []byte, orig_hash string, attr ContentAttribute) (string, int, error) {

	data, hash_changed, err := encode(orig_data, Option.EncryptKey, Option.EncryptIv, attr)
	if err != nil {
		return "", 0, err
	}

	hash := orig_hash
	if hash_changed {
		hash = c.Bucket.Sum(data)
	}

	c.queue <- uploadRequest{Filename: filename, Hash: hash, Data: data}

	return hash, len(data), nil
}

func (c *Client) AddFiles(root string) error {
	return filepath.Walk(root, func(path2 string, info os.FileInfo, err error) error {
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
		ext := filepath.Ext(path2)
		base := filepath.Base(path2)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "#") ||
			strings.HasSuffix(base, "~") ||
			ext == ".meta" || ext == ".manifest" || ext == ".tmx" || ext == ".png" {
			return nil
		}

		if path2 != "." {
			for _, pat := range ExcludePatterns {
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
				c.AddFile("", path2, info)
			} else {
				c.AddFile(root, path2[len(root)+1:], info)
			}
		} else {
			if !Option.Recursive && root != path2 {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

func (c *Client) AddFile(root string, relative string, info os.FileInfo) (bool, error) {
	b := c.Bucket

	fullPath := filepath.Join(root, relative)
	key := filepath.ToSlash(relative)
	if Option.Flatten {
		key = filepath.Base(key)
	}
	old, found := b.Contents[key]
	if found {
		b.Contents[key] = Content{
			Path:     old.Path,
			Hash:     old.Hash,
			Size:     old.Size,
			Time:     old.Time,
			OrigHash: old.OrigHash,
			OrigSize: old.OrigSize,
			Attr:     old.Attr,
			Touched:  true,
		}
		if !info.ModTime().After(old.Time) {
			return false, nil
		}
	}

	orig_data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return false, err
	}

	orig_hash := b.Sum(orig_data)
	if found && old.OrigHash == orig_hash {
		return false, nil
	}

	attr := b.GetAttribute(relative)
	hash, size, err := c.Upload(relative, orig_data, orig_hash, attr)
	if err != nil {
		return false, err
	}

	//if Verbose {
	fmt.Printf("changed file %-12s (%s)\n", relative, hash)
	//}

	b.Contents[key] = Content{
		Path:     key,
		Hash:     hash,
		Size:     size,
		Time:     info.ModTime(),
		OrigHash: orig_hash,
		OrigSize: len(orig_data),
		Attr:     attr,
		Touched:  true,
	}
	b.changed = true

	return true, nil
}

func (c *Client) Finish() error {
	b := c.Bucket

	orig_data := []byte(b.Dump())
	orig_hash := b.Sum(orig_data)

	hash, _, err := c.Upload("*bucket*", orig_data, orig_hash, DefaultContentAttribute())
	if err != nil {
		return err
	}
	b.Hash = hash

	err = ioutil.WriteFile(filepath.FromSlash(b.Path), orig_data, 0666)
	if err != nil {
		return err
	}

	ioutil.WriteFile(filepath.FromSlash(b.Path)+".hash", []byte(b.Hash), 0666)
	if Verbose {
		fmt.Printf("write bucket to '%s' (%s)\n", b.Path, b.Hash)
	}

	if b.Tag != "" {
		tag := TagFile{
			Name:       b.Tag,
			CreatedAt:  time.Now(),
			EncryptKey: Option.EncryptKey,
			EncryptIv:  Option.EncryptIv,
			Attr:       DefaultContentAttribute(),
			Hash:       b.Hash,
		}

		tagBytes, err := json.Marshal(tag)
		if err != nil {
			return err
		}

		_ = tagBytes

		/*

			_, err = b.post(path.Join("api/tags", b.Tag), tagBytes)
			if err != nil {
				return err
			}
		*/
	}

	close(c.queue)
	c.waitGroup.Wait()

	return nil
}
