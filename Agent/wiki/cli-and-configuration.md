# CLI コマンドと設定（実装メモ）

**コマンドと設定の使い方は人間向けの `docs/commands.md` と `docs/configuration.md` が正。** ここには実装の対応関係と勘所だけを書く。コマンドや設定を変えたら docs を先に直す。

## コマンドの定義場所

- `cmd/cfs/main.go` の `app.Commands` が一覧。`upload` / `sync` / `merge` / `cat` / `ls` / `config` は `main.go`、`serve` は `cmd_serve.go`、`pack` 系は `cmd_pack*.go` / `cmd_unpack.go` / `cmd_patch.go`、`gc` / `tag` / `cabinet`（`init` / `policy` / `config`）/ `keygen` は `cmd_cabinet.go`
- CLI は urfave/cli v3。コマンドのフラグは引数の後ろにも書ける（`cfs upload src -t v1`）。グローバルオプションもサブコマンドの後ろに書ける（`cfs upload -V src`）。サブコマンドの入れ子（`tag ls`、`cabinet config set`）は `Commands` で書く
- アプリの組み立ては `newApp()`（テストからプロセス内で動かすため `main` と分けた）
- 設定を使うコマンドは、先頭で `loadConfig(c)` を呼んで `*cfs.Config` を作る: `cfs.LoadConfig`（既定値 → `.cfsenv` → 環境変数）→ `-V` → `--cabinet` / `--url`（今いるフォルダ基準で `ResolveLocation`）。`--force` は `upload` が直接見る。`pack` / `unpack` / `patch` は設定を使わない
- **エラーは各コマンドの Action が `return` する**（2026-10-25 に、`check()` の panic と `os.Exit(1)` をやめた）。`main` が標準エラーに `error: ...` を出して終了コード 1 で終わる。引数の数は `needArgs`。`ls --verify` は `ng` があればエラーにする
- 終了コードは `TestMainExitCode` が、テストのバイナリを `CFS_TEST_MAIN=1` で `main` として動かして確かめている

## 設定（`internal/cfs/config.go`）

- **グローバル変数の設定はない**（2026-10-25 に `Option` / `Verbose` / `Setting` をやめた）。`Config` を `Client.Config`、`NewDownloader(location, cfg)`、`OpenStorage(location, cfg)` / `OpenFetcher(location, cfg)` に渡す。`nil` なら `DefaultConfig()`。GC は `GCOptions.Keys` / `Log` で受け取る
- ログは `Logger`（`func(format, args...)`、nil なら出さない）。`cfg.Logger()` が `Verbose` のときだけ標準出力に出すものを返す。`FileStorage.Log`、`S3Storage` も同じ
- `.cfsenv` は `json.Unmarshal` で `Config` に読む（キーの大文字・小文字は区別しない。知らないキーは無視される。v1 の `EncryptIv` や、消した `Tag` / `Repository` / `NoCache` が残っていても無視）。JSON として壊れていればエラー
- 既定値は `Recursive` / `Compress` が true、`Cabinet` は空（以前は `file:///var/cfs`）。空のままキャビネットを開くと `no cabinet` エラー
- マニフェストの読み込み（`Downloader.LoadManifest`）は、GC と同じ `decodeManifest` で圧縮・暗号化をデータから判断する。ダウンロード側の `Compress` がアップロード側と違っても読める
- `Cabinet` / `Url` の相対パスは `.cfsenv` のあるフォルダ基準で絶対パスにする（`ResolveLocation`）。Windows の `C:hoge` 形はエラー
- 環境変数 `CFS_ENCRYPT_KEY` / `CFS_OLD_ENCRYPT_KEYS`（カンマ区切り）/ `CFS_SIGNING_KEY` が `.cfsenv` より優先。`SigningKey` は `json:"-"` で `.cfsenv` からは読まない
- `-V` の設定表示は `Config.String()`（`maskedJSON`）で鍵を伏せる

## 手元のキャッシュ

| パス | 内容 |
|---|---|
| `<キャッシュ>/manifest/<sha256>` | `upload` 時のマニフェストのキャッシュ（形式 v2 の平文）。名前は キャビネット・今のフォルダ・上げるフォルダの絶対パスを NUL 区切りで SHA-256 したもの（`uploadCachePath`）。同じ場所に `<名前>.hash`（置いたマニフェストのハッシュ）。`ManifestFromFile` は読めない（v1 など）キャッシュを空として扱う |
| `<キャッシュ>/data/<hash>` | ダウンロードしたデータ（エンコード済みのまま）。`fetchVerified` がハッシュを確かめ、壊れていれば取り直す。書き込みに失敗しても（同じ中身の同時書き込みなど）ログだけ出して続ける |

`<キャッシュ>` は `CFS_CACHE_DIR`、なければ `os.UserCacheDir()/cfs`（`Config.cacheDir`）。2026-10-25 に `~/.cfs` と `~/.cfs_setting` の `HomeRoot` をやめて変えた。

## `serve`（`cmd/cfs/cmd_serve.go`。2026-10-25 に `http` から改名・作り直し）

- 用途は HTML などのアーティファクトの閲覧。認証は持たず、公開時は oauth2-proxy を前に置く前提（`../discussions/2026-10-25-public-serve-plan.md`）
- `server` 構造体が `http.Handler`。タグ → ハッシュは `--tag-ttl`（既定 30 秒）だけ覚え、ハッシュ → マニフェストは 64 件の LRU（`lruCache`）に置く。`Downloader` は起動時に 1 つ作って使い回す
- タグはサブドメイン（`<tag>.example.com`、ただし `cfs.` は無視）か、パスの先頭要素（`/<tag>/path`）から取る（`getTagAndPath`）。`ValidateTagName` を通らないものは 404。64 桁 hex はマニフェストのハッシュとして扱い、`Cache-Control: immutable` を付ける
- ファイルは `http.ServeContent` で返す（`ETag` = `OrigHash`、`If-None-Match` → 304、`Range`）。中身は一度メモリに読む（ストリーミングは未対応。計画では後回し）
- ディレクトリ一覧は `html/template` で書く（ファイル名をエスケープ）。`.htm` の Shift_JIS 特別扱いはやめた
- エラーは panic せず 404 / 500。アクセスログは `log.Printf`（`X-Forwarded-Email` があれば出す）
- `http.Server` のタイムアウト: `ReadHeaderTimeout` 10 秒、`ReadTimeout` 30 秒、`IdleTimeout` 2 分（大きいファイルのために `WriteTimeout` は付けていない）

## `--filter-cmd`（`cmd/cfs/filter.go`）

ファイルパスを改行区切りで外部コマンドの標準入力へ渡し、標準出力に返ってきた行のファイルだけを残す（`filterPaths`。マニフェストとパックで共通）。コマンド文字列はシェルを通さず `splitCommand` で分ける（`"..."` / `'...'` で空白を含められる）。コマンドが失敗したらエラー。`sync` / `merge` / `pack` / `unpack` / `pack-manifest` で使える（`patch` は使わない）。
