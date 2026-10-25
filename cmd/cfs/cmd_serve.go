package main

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
)

// cfs serve は、キャビネットの中身（HTML などのアーティファクト）をブラウザで閲覧する HTTP サーバー。
// 認証は持たないので、公開するときは oauth2-proxy などを前に置き、自身は 127.0.0.1 だけで待ち受ける。

var serveCommand = &cli.Command{
	Name:   "serve",
	Usage:  "serve cabinet contents over HTTP for browsing",
	Action: doServe,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: "127.0.0.1:8000",
			Usage: "address to listen on (put an authenticating proxy in front when exposing it)",
		},
		&cli.DurationFlag{
			Name:  "tag-ttl",
			Value: 30 * time.Second,
			Usage: "how long to cache tag lookups",
		},
	},
}

func doServe(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	downloader, err := openDownloader(cfg)
	if err != nil {
		return err
	}

	s := newServer(downloader, c.Duration("tag-ttl"))
	server := &http.Server{
		Addr:              c.String("listen"),
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	fmt.Printf("start server: %s\n", server.Addr)
	return server.ListenAndServe()
}

// server は、マニフェストをキャッシュしながらキャビネットの中身を返す
type server struct {
	downloader *cfs.Downloader
	tagTTL     time.Duration
	now        func() time.Time

	mutex     sync.Mutex
	tags      map[string]tagEntry // タグ → マニフェストのハッシュ（短い時間だけ覚える）
	manifests *lruCache           // マニフェストのハッシュ → マニフェスト（中身は変わらない）
}

type tagEntry struct {
	hash    string
	expires time.Time
}

func newServer(d *cfs.Downloader, tagTTL time.Duration) *server {
	return &server{
		downloader: d,
		tagTTL:     tagTTL,
		now:        time.Now,
		tags:       map[string]tagEntry{},
		manifests:  newLRUCache(64),
	}
}

// errNotFound は、404 を返すエラー
var errNotFound = errors.New("not found")

func (s *server) resolve(location string) (hash string, immutable bool, err error) {
	if cfs.ValidateTagName(location) != nil {
		return "", false, errNotFound
	}
	if len(location) == cfs.HashLength {
		if _, err := s.downloader.ResolveLocation(location); err == nil {
			return location, true, nil
		}
	}

	s.mutex.Lock()
	e, ok := s.tags[location]
	s.mutex.Unlock()
	if ok && s.now().Before(e.expires) {
		return e.hash, false, nil
	}

	hash, err = s.downloader.ResolveLocation(location)
	if errors.Is(err, cfs.ErrNotFound) {
		return "", false, errNotFound
	}
	if err != nil {
		return "", false, err
	}
	s.mutex.Lock()
	s.tags[location] = tagEntry{hash: hash, expires: s.now().Add(s.tagTTL)}
	s.mutex.Unlock()
	return hash, false, nil
}

func (s *server) manifest(hash string) (*cfs.Manifest, error) {
	if m, ok := s.manifests.get(hash); ok {
		return m.(*cfs.Manifest), nil
	}
	m, err := s.downloader.LoadManifest(hash)
	if errors.Is(err, cfs.ErrNotFound) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	s.manifests.put(hash, m)
	return m, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	start := s.now()
	err := s.serve(rec, r)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(rec, "404 not found", http.StatusNotFound)
		} else {
			http.Error(rec, "500 internal server error", http.StatusInternalServerError)
		}
	}
	user := r.Header.Get("X-Forwarded-Email")
	if user == "" {
		user = "-"
	}
	if err != nil && !errors.Is(err, errNotFound) {
		log.Printf("%s %s %d %s %v error: %v", r.Method, r.URL.Path, rec.status, user, s.now().Sub(start), err)
	} else {
		log.Printf("%s %s %d %s %v", r.Method, r.URL.Path, rec.status, user, s.now().Sub(start))
	}
}

func (s *server) serve(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return nil
	}

	tag, filePath := getTagAndPath(r.Host, r.URL.Path)
	if tag == "" {
		return errNotFound
	}
	hash, immutable, err := s.resolve(tag)
	if err != nil {
		return err
	}
	m, err := s.manifest(hash)
	if err != nil {
		return err
	}

	filePath = strings.Trim(filePath, "/")

	if content, ok := m.Contents[filePath]; ok && filePath != "" {
		return s.serveFile(w, r, content, immutable)
	}

	// フォルダなら、index.html があればそれを、なければ一覧を返す
	children := getDirectoryContentList(m, filePath)
	indexPath := strings.TrimLeft(filePath+"/index.html", "/")
	_, hasIndex := m.Contents[indexPath]
	if len(children) == 0 && !hasIndex {
		return errNotFound
	}
	if !strings.HasSuffix(r.URL.Path, "/") {
		// 相対リンクが正しく解決されるように、末尾に "/" を付ける
		http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
		return nil
	}
	if hasIndex {
		return s.serveFile(w, r, m.Contents[indexPath], immutable)
	}
	return renderDirectory(w, tag, filePath, children)
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request, content cfs.Content, immutable bool) error {
	data, err := s.downloader.Fetch(content.Hash, content.Attr)
	if err != nil {
		return err
	}

	mimetype := contentType(content.Path)
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", mimetype)
	h.Set("X-Content-Type-Options", "nosniff")
	// 中身はハッシュで決まるので、ETag にする
	h.Set("ETag", `"`+content.OrigHash+`"`)
	if immutable {
		// マニフェストのハッシュで指定した URL は、中身が変わらない
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// タグは指すものが変わるので、毎回確かめてもらう（変わっていなければ 304）
		h.Set("Cache-Control", "no-cache")
	}

	// ServeContent が If-None-Match（304）と Range を処理する
	http.ServeContent(w, r, path.Base(content.Path), content.Time, bytes.NewReader(data))
	return nil
}

// textTypes は、OS の設定（Windows のレジストリや /etc/mime.types）に左右されずに、
// ブラウザでそのまま読めるように返す種類。文字コードは UTF-8 とみなす
var textTypes = map[string]string{
	".md":       "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
	".txt":      "text/plain; charset=utf-8",
}

func contentType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := textTypes[ext]; ok {
		return t
	}
	return mime.TypeByExtension(ext)
}

// getTagAndPath は、ホスト名とパスから、タグとファイルのパスを取り出す
// タグはサブドメイン（<tag>.example.com。ただし cfs. は使わない）か、パスの先頭（/<tag>/path）
func getTagAndPath(host string, urlPath string) (tag string, path string) {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	var subdomain string
	if net.ParseIP(host) == nil {
		// ホスト名なので、サブドメインを使う
		if dotPos := strings.Index(host, "."); dotPos > 0 {
			subdomain = host[0:dotPos]
		}
		if subdomain == "cfs" {
			// サブドメインがcfsなら、特別にサブドメインを使用しない
			subdomain = ""
		}
	}

	tag = subdomain
	path = urlPath
	if tag == "" {
		rest := strings.TrimPrefix(urlPath, "/")
		tag, path, _ = strings.Cut(rest, "/")
		path = "/" + path
	}
	return
}

func getDirectoryContentList(b *cfs.Manifest, dir string) []cfs.Content {
	list := []cfs.Content{}
	prefix := dir
	if prefix != "" {
		prefix = prefix + "/"
	}
	for _, f := range b.Contents {
		if strings.HasPrefix(f.Path, prefix) {
			list = append(list, f)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Path < list[b].Path })
	return list
}

var directoryTemplate = template.Must(template.New("dir").Parse(`<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body>
<h1>{{.Title}}</h1>
<table>
<tr><th>Path</th><th>Size</th><th>Date</th><th>Hash</th></tr>
{{range .Files}}<tr><td><a href="{{.Href}}">{{.Name}}</a></td><td align="right">{{.Size}}</td><td>{{.Time}}</td><td>{{.Hash}}</td></tr>
{{end}}</table>
</body>
</html>
`))

type directoryFile struct {
	Href string
	Name string
	Size int
	Time string
	Hash string
}

func renderDirectory(w http.ResponseWriter, tag string, dir string, list []cfs.Content) error {
	files := make([]directoryFile, 0, len(list))
	for _, f := range list {
		name := strings.TrimLeft(f.Path[len(dir):], "/")
		files = append(files, directoryFile{
			Href: (&url.URL{Path: name}).String(),
			Name: name,
			Size: f.OrigSize,
			Time: f.Time.Format(time.RFC3339),
			Hash: f.OrigHash,
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	return directoryTemplate.Execute(w, struct {
		Title string
		Files []directoryFile
	}{Title: path.Join(tag, dir) + "/", Files: files})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// lruCache は、件数に上限のある小さな LRU キャッシュ
type lruCache struct {
	mutex sync.Mutex
	limit int
	order *list.List
	items map[string]*list.Element
}

type lruItem struct {
	key   string
	value any
}

func newLRUCache(limit int) *lruCache {
	return &lruCache{limit: limit, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lruCache) get(key string) (any, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*lruItem).value, true
}

func (c *lruCache) put(key string, value any) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if e, ok := c.items[key]; ok {
		e.Value.(*lruItem).value = value
		c.order.MoveToFront(e)
		return
	}
	c.items[key] = c.order.PushFront(&lruItem{key: key, value: value})
	for c.order.Len() > c.limit {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*lruItem).key)
	}
}
