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
	defer func() {
		err := recover()
		if err != nil {
			fmt.Printf("ERR:%v\n", err)
		}
	}()

	path := r.URL.String()[1:]
	dotPos := strings.Index(r.Host, ".")
	var subdomain string
	println(r.Host)
	if dotPos > 0 {
		subdomain = r.Host[0:dotPos]
	} else {
		slashPos := strings.Index(path, "/")
		println("a", path)
		if slashPos > 0 {
			subdomain = path[0:slashPos]
			path = path[slashPos+1:]
		}
	}
	fmt.Printf("%v|%v\n", subdomain, path)

	host := "http://cfs-autotest.s3-website-ap-northeast-1.amazonaws.com/"
	location := subdomain
	filename := path

	filename, err := url.QueryUnescape(filename)
	if err != nil {
		panic(err)
	}

	downloader, err := cfs.NewDownloader(host)
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}

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

	data, err := downloader.Fetch(content.Hash, content.Attr)
	if err != nil {
		panic(err)
	}

	w.Write(data)
}

func doHttp(c *cli.Context) {
	loadConfig(c)

	http.HandleFunc("/favicon.ico", handleStatic)
	http.HandleFunc("/", handleRoot)

	http.ListenAndServe("localhost:8000", nil)
}
