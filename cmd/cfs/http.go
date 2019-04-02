package main

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/haramako/cfs"
)

var httpCommand = cli.Command{
	Name:   "http",
	Usage:  "HTTP Server",
	Action: doHttp,
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "favicon.ico")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	println(r.URL.String())
	dotPos := strings.Index(r.Host, ".")
	var subdomain string
	if dotPos > 0 {
		subdomain = r.Host[0:dotPos]
	} else {
		subdomain = "(unknown)"
	}

	host := "http://cfs-autotest.s3-website-ap-northeast-1.amazonaws.com/"
	location := subdomain
	filename := r.URL.String()[1:]

	filename, err := url.QueryUnescape(filename)
	check(err)

	downloader, err := cfs.NewDownloader(host)
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	content, ok := bucket.Contents[filename]
	if !ok {
		fmt.Println("file " + filename + " not found")
		return
	}

	amime := mime.TypeByExtension(filepath.Ext(filename))
	if amime == "" {
		amime = "text/plain"
	}
	w.Header().Set("Content-Type", amime)

	fmt.Println(content)
	data, err := downloader.Fetch(content.Hash, content.Attr)
	check(err)

	w.Write(data)
}

func doHttp(c *cli.Context) {
	loadConfig(c)

	http.HandleFunc("/favicon.ico", handleStatic)
	http.HandleFunc("/", handleRoot)

	http.ListenAndServe("localhost:8000", nil)
}
