package cfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Manifest   *Manifest
	Storage    Storage
	Config     *Config // nil なら既定の設定
	MaxWorker  int
	waitGroup  sync.WaitGroup
	queue      chan uploadRequest
	queued     map[string]bool // 今回のアップロードで既に送ったデータ（同じ中身を2回送らない）
	cfg        *Config
	excluder   *PathPatterns
	noEncrypt  *PathPatterns
	noCompress *PathPatterns
	keys       *KeyRing
	signer     *SigningKey
	log        Logger

	errMutex sync.Mutex
	err      error // アップロードで最初に起きたエラー
}

func (c *Client) Init() error {
	if c.MaxWorker == 0 {
		c.MaxWorker = 32
	}
	c.cfg = c.Config.orDefault()
	c.log = c.cfg.Logger()

	var err error
	if c.excluder, err = NewExcluder(c.cfg.Exclude); err != nil {
		return fmt.Errorf("Exclude: %w", err)
	}
	if c.noEncrypt, err = NewPathPatterns(c.cfg.NoEncrypt); err != nil {
		return fmt.Errorf("NoEncrypt: %w", err)
	}
	if c.noCompress, err = NewPathPatterns(c.cfg.NoCompress); err != nil {
		return fmt.Errorf("NoCompress: %w", err)
	}
	if c.keys, err = c.cfg.KeyRing(); err != nil {
		return err
	}
	if c.signer, err = c.cfg.Signer(); err != nil {
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
		if c.uploadError() != nil {
			continue // 失敗したら、残りは送らずに捨てる
		}
		if err := c.Storage.Put(req.Key, req.Data, false); err != nil {
			c.errMutex.Lock()
			if c.err == nil {
				c.err = fmt.Errorf("upload %s: %w", req.Filename, err)
			}
			c.errMutex.Unlock()
		}
	}
}

func (c *Client) uploadError() error {
	c.errMutex.Lock()
	defer c.errMutex.Unlock()
	return c.err
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

	// 0 バイトのデータ（圧縮も暗号化もしない空のファイル）も、ほかと同じく置く
	key := dataKey(hash)
	if !c.queued[key] {
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

// AddFiles は、root のフォルダの中のファイルを加える。パスは root からの相対パスになる。
// 対象から外すパターン（DefaultExcludes と Config.Exclude）に合うものは加えない（フォルダなら中身ごと）。
// root にファイルを直接指定したときは、パターンによらず、そのパスで加える
func (c *Client) AddFiles(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		_, err := c.AddFile("", norm.NFC.String(root), info)
		return err
	}

	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		// OSXのためにUTF-8文字列を正規化する See: https://text.baldanders.info/golang/unicode-normalization/
		rel = norm.NFC.String(filepath.ToSlash(rel))

		if c.excluder.Match(rel, d.IsDir()) {
			c.log.Printf("excluded %s\n", rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !c.cfg.Recursive {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		_, err = c.AddFile(root, rel, info)
		return err
	})
}

func (c *Client) AddFile(root string, relative string, info os.FileInfo) (bool, error) {
	m := c.Manifest

	fullPath := filepath.Join(root, relative)
	key := c.contentKey(relative)
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

	attr := c.attribute(relative)
	hash, size, err := c.Upload(relative, origData, attr)
	if err != nil {
		return false, err
	}

	c.log.Printf("changed file %-12s (%s)\n", relative, hash)

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

	key := c.contentKey(relative)

	origHash := SumHash(content)

	attr := c.attribute(relative)
	hash, size, err := c.Upload(relative, content, attr)
	if err != nil {
		return false, err
	}

	c.log.Printf("changed file %-12s (%s)\n", relative, hash)

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

// contentKey は、ファイルのマニフェストでのパスを返す
func (c *Client) contentKey(relative string) string {
	key := filepath.ToSlash(relative)
	if c.cfg.Flatten {
		key = filepath.Base(key)
	}
	return key
}

// attribute は、ファイル（アップロードするフォルダからの相対パス）の圧縮・暗号化の属性を返す。
// 設定の既定に、NoCompress / NoEncrypt のパターンを当てる
func (c *Client) attribute(path string) ContentAttribute {
	attr := c.cfg.DefaultAttribute()
	path = filepath.ToSlash(path)
	if c.noCompress.MatchFile(path) {
		attr &^= Compressed
	}
	if c.noEncrypt.MatchFile(path) {
		attr &^= Crypted
	}
	return attr
}

// Finish は、アップロードが終わるのを待ってから、マニフェストを置く。
// 失敗したものがあれば、中身が欠けたマニフェストを置かないように、マニフェストは置かずにエラーを返す
func (c *Client) Finish() error {
	close(c.queue)
	c.waitGroup.Wait()

	if err := c.uploadError(); err != nil {
		return err
	}
	return c.UploadManifest()
}

func (c *Client) UploadManifest() error {
	m := c.Manifest

	origData := m.Dump(c.signer)

	hash, data, err := c.Encode(origData, c.cfg.DefaultAttribute())
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

		if err := os.WriteFile(filepath.FromSlash(m.Path)+".hash", []byte(m.Hash), 0666); err != nil {
			return err
		}
		c.log.Printf("write manifest to '%s' (%s)\n", m.Path, m.Hash)
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
