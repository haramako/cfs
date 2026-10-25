package cfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"
)

type uploadRequest struct {
	Filename string
	Key      string
	Data     []byte
}

type Client struct {
	Manifest  *Manifest
	Storage   Storage
	MaxWorker int
	waitGroup sync.WaitGroup
	queue     chan uploadRequest
	queued    map[string]bool // 今回のアップロードで既に送ったデータ（同じ中身を2回送らない）
	keys      *KeyRing
	signer    *SigningKey
}

func (c *Client) Init() error {
	if c.MaxWorker == 0 {
		c.MaxWorker = 32
	}

	var err error
	if c.keys, err = Option.KeyRing(); err != nil {
		return err
	}
	if c.signer, err = Option.Signer(); err != nil {
		return err
	}

	c.queue = make(chan uploadRequest, c.MaxWorker)
	c.queued = map[string]bool{}

	c.waitGroup.Add(c.MaxWorker)
	for i := 0; i < c.MaxWorker; i++ {
		go c.uploadWorker()
	}

	return nil
}

func (c *Client) uploadWorker() {
	defer c.waitGroup.Done()

	for req := range c.queue {
		// エラーのまま進むと、中身がないマニフェストを置いてしまうので止める
		err := c.Storage.Put(req.Key, req.Data, false)
		if err != nil {
			panic(err)
		}
	}
}

// Encode は、属性に従って圧縮・暗号化し、キャビネットに置くデータとそのハッシュを返す
func (c *Client) Encode(origData []byte, attr ContentAttribute) (hash string, data []byte, err error) {
	data, err = encode(origData, c.keys, attr)
	if err != nil {
		return
	}
	hash = SumHash(data)
	return
}

func (c *Client) Upload(filename string, origData []byte, attr ContentAttribute) (string, int, error) {
	hash, data, err := c.Encode(origData, attr)
	if err != nil {
		return "", 0, err
	}

	// 0 バイトのデータは置かない（ダウンロード側が空のファイルを作る）
	key := dataKey(hash)
	if len(data) > 0 && !c.queued[key] {
		c.queued[key] = true
		c.queue <- uploadRequest{Filename: filename, Key: key, Data: data}
	}

	return hash, len(data), nil
}

// ValidateCache は、手元のキャッシュ（前回のマニフェスト）を使ってよいかを確かめる。
// 前回のマニフェストがキャビネットに残っていれば、GC はその中身を消さないので、キャッシュを信用できる
func (m *Manifest) ValidateCache(s Storage) (bool, error) {
	if len(m.Contents) == 0 {
		return true, nil
	}
	if m.Hash == "" {
		return false, nil
	}
	return s.Exists(manifestKey(m.Hash))
}

func (c *Client) AddFiles(root string) error {
	return filepath.Walk(root, func(path2 string, info os.FileInfo, err error) error {
		// OSXのためにUTF-8文字列を正規化する See: https://text.baldanders.info/golang/unicode-normalization/
		path2 = norm.NFC.String(path2)

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
			ext == ".meta" || ext == ".manifest" || ext == ".tmx" {
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
	m := c.Manifest

	fullPath := filepath.Join(root, relative)
	key := filepath.ToSlash(relative)
	if Option.Flatten {
		key = filepath.Base(key)
	}
	old, found := m.Contents[key]
	if found {
		m.Contents[key] = Content{
			Path:     old.Path,
			Hash:     old.Hash,
			Size:     old.Size,
			Time:     old.Time,
			OrigHash: old.OrigHash,
			OrigSize: old.OrigSize,
			Attr:     old.Attr,
			Touched:  true,
		}
		if !info.ModTime().After(old.Time.Add(time.Second)) && info.Size() == int64(old.OrigSize) { // ファイルシステムによって誤差があるため、１秒追加する
			return false, nil
		}
	}

	origData, err := os.ReadFile(fullPath)
	if err != nil {
		return false, err
	}

	origHash := SumHash(origData)
	if found && old.OrigHash == origHash {
		old.Time = info.ModTime() // 時間だけ更新する
		old.Touched = true
		m.Contents[key] = old
		return false, nil
	}

	attr := m.GetAttribute(relative)
	hash, size, err := c.Upload(relative, origData, attr)
	if err != nil {
		return false, err
	}

	if Verbose {
		fmt.Printf("changed file %-12s (%s)\n", relative, hash)
	}

	m.Contents[key] = Content{
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

func (c *Client) AddContent(relative string, content []byte) (bool, error) {
	m := c.Manifest

	key := filepath.ToSlash(relative)
	if Option.Flatten {
		key = filepath.Base(key)
	}

	origHash := SumHash(content)

	attr := m.GetAttribute(relative)
	hash, size, err := c.Upload(relative, content, attr)
	if err != nil {
		return false, err
	}

	if Verbose {
		fmt.Printf("changed file %-12s (%s)\n", relative, hash)
	}

	m.Contents[key] = Content{
		Path:     key,
		Hash:     hash,
		Size:     size,
		Time:     time.Now(),
		OrigHash: origHash,
		OrigSize: len(content),
		Attr:     attr,
		Touched:  true,
	}

	return true, nil
}

func (c *Client) Finish() error {

	close(c.queue)
	c.waitGroup.Wait()

	err := c.UploadManifest()
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) UploadManifest() error {
	m := c.Manifest

	origData := m.Dump(c.signer)

	hash, data, err := c.Encode(origData, DefaultContentAttribute())
	if err != nil {
		return err
	}

	err = c.Storage.Put(manifestKey(hash), data, false)
	if err != nil {
		return err
	}
	m.Hash = hash

	// マニフェストの保存先が設定されているなら保存する
	if m.Path != "" {
		err = os.WriteFile(filepath.FromSlash(m.Path), origData, 0666)
		if err != nil {
			return err
		}

		os.WriteFile(filepath.FromSlash(m.Path)+".hash", []byte(m.Hash), 0666)
		if Verbose {
			fmt.Printf("write manifest to '%s' (%s)\n", m.Path, m.Hash)
		}
	}

	// タグが設定されているなら、保存する
	if m.Tag != "" {
		err = c.Storage.Put(tagKey(m.Tag), []byte(m.Hash), true)
		if err != nil {
			return err
		}
	}

	return nil
}
