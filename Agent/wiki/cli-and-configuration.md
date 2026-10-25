# CLI コマンドと設定（実装メモ）

**コマンドと設定の使い方は人間向けの `docs/commands.md` と `docs/configuration.md` が正。** ここには実装の対応関係と勘所だけを書く。コマンドや設定を変えたら docs を先に直す。

## コマンドの定義場所

- `cmd/cfs/main.go` の `app.Commands` が一覧。`upload` / `sync` / `merge` / `cat` / `ls` / `config` / `setting` は `main.go`、`http` は `cmd_http.go`、`pack` 系は `cmd_pack*.go` / `cmd_unpack.go` / `cmd_patch.go`、`gc` / `tag` / `cabinet` / `keygen` は `cmd_cabinet.go`
- CLI は urfave/cli v3。コマンドのフラグは引数の後ろにも書ける（`cfs upload src -t v1`）。グローバルオプションもサブコマンドの後ろに書ける（`cfs upload -V src`）。サブコマンドの入れ子（`tag ls`、`cabinet config set`）は `Commands` で書く
- 各コマンドの先頭で `loadConfig` を呼ぶ: `InitDefaultSetting` → `-V` → `LoadDefaultOptions`（`.cfsenv` → 環境変数）→ `--cabinet` / `--url`（今いるフォルダ基準で `ResolveLocation`）→ `--force`
- エラーの多くは `check(err)` で panic する（スタックトレースが出る）。利用者向けのエラー（タグ名、設定の誤りなど）は `fmt.Println` + `os.Exit(1)`

## 設定の読み込み（`internal/cfs/option.go`）

- `.cfsenv` は `json.Unmarshal` で `Option` に読む（キーの大文字・小文字は区別しない。知らないキーは無視される。v1 の `EncryptIv` が残っていても無視）
- `Cabinet` / `Url` の相対パスは `.cfsenv` のあるフォルダ基準で絶対パスにする（`ResolveLocation`）。Windows の `C:hoge` 形はエラー
- 環境変数 `CFS_ENCRYPT_KEY` / `CFS_OLD_ENCRYPT_KEYS`（カンマ区切り）/ `CFS_SIGNING_KEY` が `.cfsenv` より優先。`SigningKey` は `json:"-"` で `.cfsenv` からは読まない
- `-V` の設定表示は `maskedJSON` で鍵を伏せる
- `Tag` / `Repository` は構造体にあるが未使用

## 手元のキャッシュ

| パス | 内容 |
|---|---|
| `~/.cfs/cache/<md5>` | `upload` 時のマニフェストのキャッシュ（形式 v2 の平文）。名前は `Cabinet$cwd$絶対パス...` をエスケープして MD5 したもの（内容アドレスではないので MD5 のまま）。同じ場所に `<md5>.hash`（置いたマニフェストのハッシュ）。`ManifestFromFile` は読めない（v1 など）キャッシュを空として扱う |
| `~/.cfs/datacache/<hash>` | ダウンロードしたデータ（エンコード済みのまま）。`fetchObject` がハッシュを確かめ、壊れていれば取り直す |

`~/.cfs` の場所は `~/.cfs_setting`（YAML）の `HomeRoot` で変えられる（`config.go`）。

## `http` サーバー（`cmd/cfs/cmd_http.go`）

- タグはサブドメイン（`<tag>.example.com`、ただし `cfs.` は無視）か、パスの先頭要素（`/<tag>/path`）から取る
- ファイルがあれば返し、なければ `index.html`、それもなければディレクトリ一覧を返す
- ダウンロード元は `getDownloaderURL()`（`Url`、なければキャビネット）
- 公開向けではない（XSS、panic、localhost 固定、毎回マニフェストを取り直す）。`../discussions/2026-10-25-public-serve-plan.md` で `cfs serve` として作り直す予定

## `--filter-cmd`（`cmd/cfs/filter.go`）

ファイルパスを改行区切りで外部コマンドの標準入力へ渡し、標準出力に返ってきた行のファイルだけを残す。コマンド文字列は単純に空白で分割されるので、引数のクォートはできない。`sync` / `merge` / `pack` / `unpack` / `pack-manifest` で使える（`patch` は使わない）。
