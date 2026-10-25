package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

// cfs console は、キャビネットのタグとマニフェストの中身を見るための、手元向けの管理画面。
// 今の設定の鍵で復号した中身を見せるので、ループバックのアドレスでだけ待ち受け、Host が localhost のもの以外は断る
// （DNS リバインディングで、ほかのサイトから読まれないように）。中身は見るだけで、変更はしない

var consoleCommand = &cli.Command{
	Name:   "console",
	Usage:  "open a local web console to browse tags and manifests of the cabinet",
	Action: doConsole,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Value: "127.0.0.1:8001",
			Usage: "address to listen on (loopback only)",
		},
	},
}

func doConsole(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	if err := checkLoopback(c.String("listen")); err != nil {
		return err
	}
	con, err := newConsole(cfg)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              c.String("listen"),
		Handler:           con,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("cfs console: http://%s/\n", server.Addr)
	return server.ListenAndServe()
}

// checkLoopback は、待ち受けのアドレスがループバックかを確かめる
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("console listens only on a loopback address (like 127.0.0.1:8001), not %q", addr)
}

// isLocalHost は、リクエストの Host が、手元のアドレスかを返す
func isLocalHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type console struct {
	cfg     *cfs.Config
	storage cfs.Storage
	fetcher cfs.Fetcher
	files   *server // ファイルの中身を返す（cfs serve と同じ）
	mux     *http.ServeMux
}

func newConsole(cfg *cfs.Config) (*console, error) {
	storage, fetcher, err := openCabinet(cfg)
	if err != nil {
		return nil, err
	}
	downloader, err := cfs.NewDownloader(cfg.Cabinet, cfg)
	if err != nil {
		return nil, err
	}
	con := &console{
		cfg:     cfg,
		storage: storage,
		fetcher: fetcher,
		files:   newServer(downloader, 0), // タグはいつも読み直す
		mux:     http.NewServeMux(),
	}
	con.mux.HandleFunc("GET /{$}", con.handleIndex)
	con.mux.HandleFunc("GET /tag/{name}", con.handleTag)
	con.mux.HandleFunc("GET /manifest/{hash}", con.handleManifest)
	con.mux.Handle("GET /files/", http.StripPrefix("/files", con.files))
	return con, nil
}

func (con *console) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLocalHost(r.Host) {
		http.Error(w, "403 forbidden (console accepts only localhost)", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	con.mux.ServeHTTP(w, r)
}

// tagRow は、タグの一覧の 1 行
type tagRow struct {
	cfs.TagInfo
	Files     int
	OrigSize  int64
	Size      int64
	SignedBy  string
	LoadError string
}

func (con *console) handleIndex(w http.ResponseWriter, r *http.Request) {
	tags, err := cfs.ListTags(con.storage, con.fetcher)
	if err != nil {
		con.fail(w, err)
		return
	}
	cabinetCfg, err := cfs.LoadCabinetConfig(con.fetcher)
	if err != nil {
		con.fail(w, err)
		return
	}
	retention, _ := cabinetCfg.RetentionDuration()

	// マニフェストを並列に読み、ファイルの数と大きさを数える（読めないものは、その行にエラーを出す）
	rows := make([]tagRow, len(tags))
	var eg errgroup.Group
	eg.SetLimit(8)
	for i, t := range tags {
		rows[i].TagInfo = t
		eg.Go(func() error {
			m, err := con.files.manifest(t.Hash)
			if err != nil {
				rows[i].LoadError = err.Error()
				return nil
			}
			rows[i].Files = len(m.Contents)
			rows[i].OrigSize, rows[i].Size = manifestSizes(m)
			rows[i].SignedBy = m.SignedBy
			return nil
		})
	}
	eg.Wait()
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	con.render(w, indexTemplate, map[string]any{
		"Cabinet":   con.cfg.Cabinet,
		"Format":    cabinetCfg.Format,
		"Retention": retention,
		"Encrypted": con.cfg.EncryptKey != "",
		"Tags":      rows,
	})
}

func (con *console) handleTag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	hash, _, err := con.files.resolve(name)
	if err != nil {
		con.fail(w, err)
		return
	}
	con.renderManifest(w, name, hash)
}

func (con *console) handleManifest(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if len(hash) != cfs.HashLength {
		con.fail(w, errNotFound)
		return
	}
	con.renderManifest(w, "", hash)
}

// fileRow は、マニフェストのファイルの一覧の 1 行
type fileRow struct {
	cfs.Content
	Href string
}

func (con *console) renderManifest(w http.ResponseWriter, tag string, hash string) {
	m, err := con.files.manifest(hash)
	if err != nil {
		con.fail(w, err)
		return
	}
	rows := make([]fileRow, 0, len(m.Contents))
	for _, c := range m.Contents {
		rows = append(rows, fileRow{Content: c, Href: "/files/" + hash + "/" + (&url.URL{Path: c.Path}).EscapedPath()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	origSize, size := manifestSizes(m)

	con.render(w, manifestTemplate, map[string]any{
		"Tag":      tag,
		"Hash":     hash,
		"SignedBy": m.SignedBy,
		"Files":    rows,
		"OrigSize": origSize,
		"Size":     size,
	})
}

// manifestSizes は、マニフェストのファイルの元の大きさと、キャビネットに置いた大きさの合計を返す
func manifestSizes(m *cfs.Manifest) (orig int64, stored int64) {
	for _, c := range m.Contents {
		orig += int64(c.OrigSize)
		stored += int64(c.Size)
	}
	return
}

func (con *console) render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := t.Execute(w, data); err != nil {
		log.Printf("console: %v", err)
	}
}

func (con *console) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, errNotFound) || errors.Is(err, cfs.ErrNotFound) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	log.Printf("console: %v", err)
	http.Error(w, "500 internal server error: "+err.Error(), http.StatusInternalServerError)
}

// humanSize は、バイト数を読みやすくする
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

var consoleFuncs = template.FuncMap{
	"size":  humanSize,
	"isize": func(n int) string { return humanSize(int64(n)) },
	"short": func(h string) string { return h[:min(12, len(h))] },
	"time":  func(t time.Time) string { return t.Local().Format("2006-01-02 15:04:05") },
	"attr": func(a cfs.ContentAttribute) []string {
		var s []string
		if a.Compressed() {
			s = append(s, "zlib")
		}
		if a.Crypted() {
			s = append(s, "enc")
		}
		if len(s) == 0 {
			return []string{"raw"}
		}
		return s
	},
}

// consoleLayout は、各ページに共通の見た目
const consoleLayout = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{template "title" .}} - cfs console</title>
<style>
:root { --bg: #fafafa; --fg: #1f2328; --muted: #656d76; --line: #d0d7de; --head: #f0f2f4; --link: #0969da; --err: #cf222e; --badge: #eaeef2; }
@media (prefers-color-scheme: dark) {
  :root { --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0; --line: #30363d; --head: #161b22; --link: #4493f8; --err: #f85149; --badge: #21262d; }
}
body { margin: 0; padding: 16px 24px; background: var(--bg); color: var(--fg); font: 14px/1.5 system-ui, sans-serif; }
a { color: var(--link); text-decoration: none; }
a:hover { text-decoration: underline; }
h1 { font-size: 20px; margin: 0 0 4px; }
.sub { color: var(--muted); margin-bottom: 16px; word-break: break-all; }
.sub span { margin-right: 16px; }
input { padding: 6px 8px; width: min(400px, 100%); border: 1px solid var(--line); border-radius: 6px; background: var(--bg); color: var(--fg); margin-bottom: 8px; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: 4px 8px; border-bottom: 1px solid var(--line); vertical-align: top; }
th { background: var(--head); position: sticky; top: 0; }
td.num, th.num { text-align: right; white-space: nowrap; }
.mono { font-family: ui-monospace, monospace; }
.muted { color: var(--muted); }
.err { color: var(--err); }
.badge { background: var(--badge); border-radius: 4px; padding: 0 6px; font-size: 12px; }
</style>
</head>
<body>
{{template "body" .}}
<script>
document.querySelectorAll("input[data-filter]").forEach(function (input) {
  var rows = document.querySelectorAll(input.dataset.filter + " tbody tr");
  input.addEventListener("input", function () {
    var q = input.value.toLowerCase();
    rows.forEach(function (row) { row.hidden = q !== "" && row.textContent.toLowerCase().indexOf(q) < 0; });
  });
});
</script>
</body>
</html>
`

var indexTemplate = template.Must(template.Must(template.New("layout").Funcs(consoleFuncs).Parse(consoleLayout)).Parse(`
{{define "title"}}Tags{{end}}
{{define "body"}}
<h1>cfs console</h1>
<div class="sub">
<span>Cabinet: <span class="mono">{{.Cabinet}}</span></span>
<span>Format: {{if .Format}}{{.Format}}{{else}}(not written){{end}}</span>
<span>GC retention: {{.Retention}}</span>
<span>Encrypt key: {{if .Encrypted}}set{{else}}not set{{end}}</span>
</div>
<input data-filter="#tags" placeholder="Filter tags">
<table id="tags">
<thead><tr><th>Tag</th><th>Manifest</th><th class="num">Files</th><th class="num">Size</th><th class="num">Stored</th><th>Signed</th><th>Updated</th></tr></thead>
<tbody>
{{range .Tags}}<tr>
<td><a href="/tag/{{.Name}}">{{.Name}}</a></td>
<td class="mono"><a href="/manifest/{{.Hash}}" title="{{.Hash}}">{{short .Hash}}</a></td>
{{if .LoadError}}<td colspan="4" class="err">{{.LoadError}}</td>{{else}}<td class="num">{{.Files}}</td><td class="num">{{size .OrigSize}}</td><td class="num">{{size .Size}}</td><td class="mono">{{if .SignedBy}}{{.SignedBy}}{{else}}<span class="muted">-</span>{{end}}</td>{{end}}
<td>{{time .LastModified}}</td>
</tr>
{{else}}<tr><td colspan="7" class="muted">No tags</td></tr>
{{end}}</tbody>
</table>
{{end}}
`))

var manifestTemplate = template.Must(template.Must(template.New("layout").Funcs(consoleFuncs).Parse(consoleLayout)).Parse(`
{{define "title"}}{{if .Tag}}{{.Tag}}{{else}}{{short .Hash}}{{end}}{{end}}
{{define "body"}}
<div><a href="/">&larr; Tags</a></div>
<h1>{{if .Tag}}{{.Tag}}{{else}}Manifest {{short .Hash}}{{end}}</h1>
<div class="sub">
<span>Manifest: <span class="mono">{{.Hash}}</span></span>
<span>{{len .Files}} files</span>
<span>Size: {{size .OrigSize}} (stored {{size .Size}})</span>
<span>Signed: {{if .SignedBy}}<span class="mono">{{.SignedBy}}</span>{{else}}no{{end}}</span>
</div>
<input data-filter="#files" placeholder="Filter files">
<table id="files">
<thead><tr><th>Path</th><th class="num">Size</th><th class="num">Stored</th><th>Attr</th><th>Modified</th><th>Hash</th></tr></thead>
<tbody>
{{range .Files}}<tr>
<td class="mono"><a href="{{.Href}}">{{.Path}}</a></td>
<td class="num">{{isize .OrigSize}}</td>
<td class="num">{{isize .Size}}</td>
<td>{{range attr .Attr}}<span class="badge">{{.}}</span> {{end}}</td>
<td>{{time .Time}}</td>
<td class="mono" title="{{.OrigHash}}">{{short .OrigHash}}</td>
</tr>
{{else}}<tr><td colspan="6" class="muted">No files</td></tr>
{{end}}</tbody>
</table>
{{end}}
`))
