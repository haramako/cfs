package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
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

func (b *Bucket) Finish() error {
	orig_data := []byte(b.Dump())
	orig_hash := b.Sum(orig_data)

	hash, _, err := b.Upload("*bucket*", orig_data, orig_hash, DefaultContentAttribute())
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

		_, err = b.post(path.Join("api/tags", b.Tag), tagBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func isHash(str string) bool {
	if len(str) != 32 {
		return false
	}
	for _, c := range str {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func (b *Bucket) Upload(filename string, orig_data []byte, orig_hash string, attr ContentAttribute) (string, int, error) {
	if !isHash(orig_hash) {
		return "", 0, fmt.Errorf("invalid hash %s", orig_hash)
	}

	data := orig_data
	hash := orig_hash
	hash_changed := false

	if attr.Compressed() {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(orig_data)
		w.Close()
		data = buf.Bytes()
		hash_changed = true
	}

	if attr.Crypted() {
		block, err := aes.NewCipher([]byte(Option.EncryptKey))
		if err != nil {
			panic(err)
		}
		cfb := cipher.NewCFBEncrypter(block, []byte(Option.EncryptIv))
		cipher_data := make([]byte, len(data))
		cfb.XORKeyStream(cipher_data, data)
		data = cipher_data
		hash_changed = true
	}

	if hash_changed {
		hash = b.Sum(data)
	}

	err := b.uploadFile(filename, hash, data, false)
	if err != nil {
		return "", 0, err
	}

	return hash, len(data), nil
}

func (b *Bucket) Decode(data []byte, attr ContentAttribute) ([]byte, error) {

	if attr.Compressed() {
		var buf bytes.Buffer
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		r.Read(data)
		r.Close()
		data = buf.Bytes()
	}

	if attr.Crypted() {
		block, err := aes.NewCipher([]byte(Option.EncryptKey))
		if err != nil {
			return nil, err
		}
		cfb := cipher.NewCFBEncrypter(block, []byte(Option.EncryptIv))
		cipher_data := make([]byte, len(data))
		cfb.XORKeyStream(cipher_data, data)
		data = cipher_data
	}

	return data, nil
}

func (b *Bucket) post(location string, body []byte) ([]byte, error) {
	cli := &http.Client{}

	url, err := b.CabinetUrl.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("cannot parse url %s", location)
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cannot create request %s", url.String())
	}

	req.SetBasicAuth(Option.AdminUser, Option.AdminPass)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error request to %s, %s", url.String(), err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response code not 200 OK but %d, %s", resp.StatusCode, url.String())
	}

	resp_body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read body %s", url.String())
	}

	return resp_body, nil
}

func (b *Bucket) uploadFile(filename string, hash string, body []byte, overwrite bool) error {

	nonexists_res, err := b.post("api/nonexists", []byte(hash))
	if err != nil {
		return err
	}

	if len(nonexists_res) == 0 {
		return nil
	}

	_, err = b.post(path.Join("api/upload", hash), body)
	if err != nil {
		return err
	}

	b.UploadCount++

	//if Verbose {
	fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	//}

	return nil
}

func (b *Bucket) GetAttribute(path string) ContentAttribute {
	var attr = DefaultContentAttribute()
	// TODO: とりあえずフィルタを固定している
	if filepath.Ext(path) == ".ab" || filepath.Ext(path) == ".raw" || filepath.Ext(path) == ".pbx" || filepath.Ext(path) == ".mp4" {
		attr = ContentAttribute(0)
	}
	return attr
}

func (b *Bucket) AddFile(root string, relative string, info os.FileInfo) (bool, error) {
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
	hash, size, err := b.Upload(relative, orig_data, orig_hash, attr)
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

func (b *Bucket) AddFiles(root string) error {
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

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
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
