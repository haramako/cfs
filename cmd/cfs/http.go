package main

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli"
	"github.com/haramako/cfs"
)

var httpCommand = cli.Command{
	Name:   "http",
	Usage:  "HTTP Server",
	Action: doHTTP,
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "favicon.ico")
}

func getTagAndPath(host string, urlPath string) (tag string, path string) {
	// サブドメインを取得する
	var subdomain string
	var ip = net.ParseIP(host)
	if ip == nil {
		// ホスト名なので、サブドメインを使う
		dotPos := strings.Index(host, ".")
		if dotPos > 0 {
			subdomain = host[0:dotPos]
		}

		if subdomain == "cfs" {
			// サブドメインがcfsなら、特別にサブドメインを使用しない
			subdomain = ""
		}
	}

	// タグを取得する
	tag = subdomain
	path = urlPath
	if tag == "" {
		slashPos := strings.Index(path[1:], "/")
		if slashPos > 0 {
			tag = path[1 : slashPos+1]
			path = path[slashPos+1:] // 頭の"/"の分１足す
		}
	}

	return
}

func getDirectoryContentList(b *cfs.Bucket, path string) []cfs.Content {
	list := []cfs.Content{}
	if path != "" {
		path = path + "/"
	}
	for _, f := range b.Contents {
		if strings.HasPrefix(f.Path, path) {
			list = append(list, f)
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Path < list[b].Path })
	return list
}

func renderDirectory(w http.ResponseWriter, path string, list []cfs.Content) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html lang=ja>"))
	s := "<table><tr><th>Path</td><td>Size</td><td>Date</td><td>Hash</td></tr>"
	w.Write([]byte(s))
	for _, f := range list {
		cpath := strings.TrimLeft(f.Path[len(path):], "/")
		s := fmt.Sprintf("<tr><td><a href='%v'>%v</a></td><td align=right>%v</td><td>%v</td><td>%v</td></tr>", cpath, cpath, f.OrigSize, f.Time, f.Hash)
		w.Write([]byte(s))
	}
	w.Write([]byte("</table>"))
}

func renderFile(w http.ResponseWriter, downloader *cfs.Downloader, content cfs.Content) {
	data, err := downloader.Fetch(content.Hash, content.Attr)
	if err != nil {
		panic(err)
	}
	// MIMEを設定する
	mimetype := mime.TypeByExtension(filepath.Ext(content.Path))
	if mimetype == "" {
		mimetype = "text/plain"
	}

	// MEMO:Excelの特殊対応, ExcelのHTMLがSJISで出力されるので、SJISに固定する
	if strings.HasSuffix(content.Path, ".htm") {
		mimetype = "text/html; charset=shift_jis"
	}

	w.Header().Set("Content-Type", mimetype)
	println(content.Path, mimetype)

	w.Write(data)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {

	r.Host = "td-doc.hoge"

	println(r.URL.String())
	urlPath, err := url.QueryUnescape(r.URL.String())
	if err != nil {
		panic(err)
	}

	tag, path := getTagAndPath(r.Host, urlPath)

	fmt.Printf("Loading tag: %v, path: %v\n", tag, path)

	host := cfs.Option.Url
	location := tag
	path = path[1:]

	downloader, err := cfs.NewDownloader(host)
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}

	// "/"で終わる場合は、"/"をなくす
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}

	content, ok := bucket.Contents[path]
	if ok {
		renderFile(w, downloader, content)
		return
	}

	// index.htmlがあればそれを返す
	indexPath := strings.TrimLeft(strings.TrimRight(path, "/")+"/index.html", "/")
	content, ok = bucket.Contents[indexPath]
	if ok {
		renderFile(w, downloader, content)
		return
	}

	// ディレクトリであれば、ファイル一覧を返す
	children := getDirectoryContentList(bucket, path)
	fmt.Printf("getDirectoryContentList %v", path)
	if len(children) > 0 {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
			return
		}
		renderDirectory(w, path, children)
		return
	}

	panic(fmt.Errorf("file or directory %v not found", path))
}

func doHTTP(c *cli.Context) {
	loadConfig(c)

	http.HandleFunc("/favicon.ico", handleStatic)
	http.HandleFunc("/", handleRoot)

	http.ListenAndServe("localhost:8000", nil)
}
