# CLI コマンドと設定

README の `cfs -h` は古い。実際のコマンドは `cmd/cfs/main.go` の `app.Commands` が正。

CLI は urfave/cli v3。v1 時代と同じく、コマンドのフラグは引数の後ろにも書ける（`cfs upload src -t v1`）。v3 ではグローバルオプションをサブコマンドの後ろにも書ける（`cfs upload -V src`）。ヘルプの書式は v1 と異なり、不正なフラグのエラーは標準エラーに出る。

## グローバルオプション

| オプション | 既定値 | 内容 |
|---|---|---|
| `--verbose, -V` | false | 詳細ログ（`cfs.Verbose`） |
| `--config, -C` | `.cfsenv` | プロジェクト設定ファイル |
| `--setting, -S` | `.cfs_setting` | **定義のみで未使用**（実際は常に `~/.cfs_setting`） |
| `--cabinet, -c` | — | `Option.Cabinet` を上書き |
| `--force, -f` | false | ローカルのバケットキャッシュを使わずに全ファイルを再判定（`Option.NoCache`） |
| `--filter-cmd` | — | ファイル絞り込み用の外部コマンド（後述） |

`loadConfig` は `c.GlobalString("url")` も読むが、`--url` フラグは定義されていないので常に空。ダウンロード URL を変えたいときは `.cfsenv` の `Url` を使う。

## コマンド一覧

| コマンド | 引数 | 内容 |
|---|---|---|
| `upload` | `[-t tag] [-b bucket-file] [-o hash-out] [dir ...]` | ディレクトリをアップロード。dir 省略時は `.`。`-o` でバケットハッシュをファイルに書き出す |
| `sync` | `location output-dir` | バケットの全ファイルを出力先へ書き出す |
| `merge` | `[-o hash-out] output-tag location [...]` | 複数バケットをマージして新しいバケット/タグを作る。後ろの location が優先。データは再アップロードしない |
| `cat` | `location filename` | 1 ファイルを標準出力へ（デバッグ用） |
| `ls` | `[--verify] location` | ファイル一覧（パス, OrigHash, OrigSize, 時刻）。`--verify` で各データの存在を 32 並列の HEAD で確認し ok/ng を表示 |
| `config` | — | 現在の Cabinet とダウンロード URL を表示 |
| `setting` | — | 現在の `HomeRoot` を表示 |
| `http` | `[-p port]` | バケットの中身を閲覧できる HTTP サーバー（`localhost:8000`） |
| `pack` | `packfile dir` | ディレクトリをパックファイル化 |
| `unpack` | `packfile [-o dir]` | パックファイルを展開。`-o` なしなら一覧表示 |
| `pack-bucket` | `location packfile` | バケットの中身をダウンロードしてパックファイル化 |
| `patch` | `base.tp current.tp output.tp` | 2 つのパックの差分パックを作る |

pack 系の詳細は [pack-format.md](pack-format.md)。

### `--filter-cmd`

`sync` / `merge` / `pack` / `unpack` / `pack-bucket` で使える。ファイルパスを改行区切りで外部コマンドの標準入力へ渡し、標準出力に返ってきた行のファイルだけを残す（`cmd/cfs/filter.go`）。コマンド文字列は単純に空白で分割されるので、引数のクォートはできない。

### `http` サーバー

- タグはサブドメイン（`<tag>.example.com`、ただし `cfs.` は無視）か、パスの先頭要素（`/<tag>/path`）から取る
- ファイルがあれば返し、なければ `index.html`、それもなければディレクトリ一覧を返す
- `.htm` は Excel 出力の HTML 用に `text/html; charset=shift_jis` 固定
- ダウンロード元は `Option.Url` を直接使う（`getDownloaderURL` を通らない）ので、`.cfsenv` に `Url` が必要
- エラー時は panic（net/http が recover して接続を切る）

## 設定ファイル

### `.cfsenv`（プロジェクト単位、JSON）

カレントディレクトリの `.cfsenv`（`-C` で変更可）を `json.Unmarshal` で `cfs.Option` に読む。キーは大文字小文字を区別しない。ファイルがなければ既定値のまま。

```json
{
  "Cabinet": "s3://my-bucket",
  "Url": "https://cdn.example.com/",
  "Compress": true,
  "EncryptKey": "12345678901234567890123456789012",
  "EncryptIv": "1234567890123456",
  "Recursive": true,
  "Flatten": false
}
```

| キー | 既定値 | 内容 |
|---|---|---|
| `Cabinet` | `file:///var/cfs` | アップロード先 URL |
| `Url` | — | ダウンロード元 URL。空なら Cabinet から導出 |
| `Compress` | true | zlib 圧縮するか |
| `EncryptKey` / `EncryptIv` | 空 | AES 鍵/IV。鍵が空なら暗号化しない |
| `Recursive` | true | サブディレクトリも走査するか |
| `Flatten` | false | パスを捨ててファイル名だけをキーにする |
| `NoCache` | false | `--force` と同じ |
| `Tag` / `Repository` | — | 構造体にあるが未使用 |

### `~/.cfs_setting`（ユーザー単位、YAML）

```yaml
HomeRoot: D:/somewhere
```

`HomeRoot` を変えると `.cfs` ディレクトリの場所が `<HomeRoot>/.cfs` になる（既定はユーザーのホーム）。2020-12 に追加された機能。

### ローカルのキャッシュディレクトリ

| パス | 内容 |
|---|---|
| `~/.cfs/cache/<md5>` | `upload` 時のバケットキャッシュ。名前は `Cabinet$cwd$絶対パス...` をエスケープして MD5 したもの。同じ場所に `<md5>.hash` も置かれる |
| `~/.cfs/datacache/<hash>` | ダウンロードしたデータ（エンコード済みのまま）。消しても再取得されるだけ |

`-b` でバケットファイルを明示すると、キャッシュディレクトリの代わりにそこを使う。
