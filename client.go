package cfs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		}
	}
}

func (c *Client) Encode(origHash string, origData []byte, attr ContentAttribute) (hash string, data []byte, err error) {
	data, hashChanged, err := encode(origData, Option.EncryptKey, Option.EncryptIv, attr)
	if err != nil {
		return
	}

	if hashChanged {
		hash = c.Bucket.Sum(data)
	} else {
		hash = origHash
	}

	return
}

func (c *Client) Upload(filename string, origHash string, origData []byte, attr ContentAttribute) (string, int, error) {
	hash, data, err := c.Encode(origHash, origData, attr)
	if err != nil {
		return "", 0, err
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

	origData, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return false, err
	}

	origHash := b.Sum(origData)
	if found && old.OrigHash == origHash {
		return false, nil
	}

	attr := b.GetAttribute(relative)
	hash, size, err := c.Upload(relative, origHash, origData, attr)
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
		OrigHash: origHash,
		OrigSize: len(origData),
		Attr:     attr,
		Touched:  true,
	}

	return true, nil
}

func (c *Client) Finish() error {
	err := c.UploadBucket()
	if err != nil {
		return err
	}

	close(c.queue)
	c.waitGroup.Wait()

	return nil
}

func (c *Client) UploadBucket() error {
	b := c.Bucket

	origData := []byte(b.Dump())
	origHash := b.Sum(origData)

	hash, data, err := c.Encode(origHash, origData, DefaultContentAttribute())
	if err != nil {
		return err
	}

	err = c.Storage.Upload("*bucket*", hash, data, false)
	if err != nil {
		return err
	}
	b.Hash = hash

	// バケットの保存先が設定されているなら保存する
	if b.Path != "" {
		err = ioutil.WriteFile(filepath.FromSlash(b.Path), origData, 0666)
		if err != nil {
			return err
		}

		ioutil.WriteFile(filepath.FromSlash(b.Path)+".hash", []byte(b.Hash), 0666)
		if Verbose {
			fmt.Printf("write bucket to '%s' (%s)\n", b.Path, b.Hash)
		}
	}

	// タグが設定されているなら、保存する
	if b.Tag != "" {
		err = c.Storage.UploadTag(b.Tag, []byte(b.Hash))
		if err != nil {
			return err
		}
	}

	return nil
}
