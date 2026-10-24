# アーキテクチャ概要

CFS (Content base File System) は、ゲームアセットのような大量ファイルを **内容ハッシュ (MD5) 単位で差分アップロード／ダウンロード** する Go 製ツール。git のオブジェクトストアに近い構造で、同一内容のファイルはバージョンやプラットフォームをまたいで 1 つに共有される。

- モジュール: `github.com/haramako/cfs`（`go.mod` は go 1.26）
- 成果物: CLI `cfs`（`cmd/cfs`）のみ。実装は `internal/` にあり、外部からライブラリとしては使えない（2026-10 に CLI 専用と決定）
- 開発期間: 2015-09 開始、2021-06 以降は休眠、2026-10 に構成と依存を現代化

## パッケージ構成

`internal/cfs/` がコア実装（package `cfs`）、`internal/pack/` がパック形式、`cmd/cfs/` が CLI。

| パス（`internal/cfs/` 内） | 役割 |
|---|---|
| `bucket.go` | `Bucket`（ファイル一覧＝マニフェスト）と `Content`（1 ファイル分のメタ）、`ContentAttribute`（圧縮/暗号フラグ）、バケットの TSV シリアライズ |
| `client.go` | アップロード側。ディレクトリ走査 → 変更検出 → エンコード → ワーカーで並列アップロード → バケット/タグのアップロード |
| `downloader.go` | ダウンロード側。タグ解決 → バケット取得 → データ取得（ローカルキャッシュ経由）→ デコード |
| `storage.go` | `Storage` インターフェースと URL スキームからの生成（`file://` / `gs://` / `s3://`） |
| `file_storage.go` / `gcs_storage.go` / `s3_storage.go` | 各バックエンドの書き込み実装 |
| `dummy_storage.go` | テスト用のメモリ内ストレージ |
| `util.go` | ハッシュ判定、`xx/yyyy…` パス変換、zlib 圧縮 + AES-CFB 暗号の `encode`/`decode` |
| `option.go` | プロジェクト単位の設定 `.cfsenv`（JSON）→ グローバル変数 `Option` |
| `config.go` | ユーザー単位の設定 `~/.cfs_setting`（YAML）とキャッシュディレクトリ（`~/.cfs/…`） |
| `internal/pack/` | 独自バイナリ形式のパックファイル（`TP` 形式）。詳細は [pack-format.md](pack-format.md) |
| `cmd/cfs/` | CLI（urfave/cli v3）。コマンド一覧は [cli-and-configuration.md](cli-and-configuration.md) |

主な外部依存: S3 は aws-sdk-go-v2、GCS は cloud.google.com/go/storage、YAML は go.yaml.in/yaml/v3、atomic な書き込みに natefinch/atomic。

## 書き込みと読み込みの非対称性

**書き込み（アップロード）は `Storage` 実装経由、読み込み（ダウンロード）は常に HTTP(S)/`file://` の GET** という設計になっている。

- `Storage` インターフェースは `Upload` / `UploadTag` / `DownloaderUrl` の 3 メソッドだけで、読み込み API を持たない
- `Downloader` は `BaseUrl` に対して `data/xx/yyyy…` と `tag/<name>` を GET するだけ。`file://` は `http.NewFileTransport` で扱う
- ダウンロード URL は `.cfsenv` の `Url` で明示するか、未指定なら `Storage.DownloaderUrl()`（例: GCS なら `http://storage.googleapis.com/<bucket>/`）を使う（`cmd/cfs/main.go` の `getDownloaderURL`）
- そのため GCS/S3 の場合、**バケットは公開読み取り可能（S3 は静的ウェブサイトホスティング）であることが前提**。CDN やプロキシを前段に置く想定

## アップロードの流れ（`cfs upload`）

1. `.cfsenv` を読み、ローカルのバケットキャッシュ（前回アップロード時のマニフェスト）を `~/.cfs/cache/<md5>` からロード
2. `Client.AddFiles` で対象ディレクトリを走査（除外ルールは [known-issues-and-gotchas.md](known-issues-and-gotchas.md) 参照）
3. `Client.AddFile` で変更検出
   - 前回の `Time + 1秒` 以前の mtime かつサイズ一致 → 読まずにスキップ
   - 読み込んで MD5（`OrigHash`）が一致 → 時刻だけ更新
   - それ以外 → `encode`（圧縮→暗号化）してキューに積む
4. 32 並列の `uploadWorker` が `Storage.Upload` を呼ぶ（既存ハッシュはバックエンド側でスキップ）
5. `RemoveUntouched` で今回見つからなかったエントリを削除（バケット＝ディレクトリの完全なスナップショット）
6. `Client.Finish` → `UploadBucket`: バケット自体もエンコードして `data/` に置き、そのハッシュをタグ `tag/<name>` に書く。ローカルキャッシュと `<cache>.hash` も更新

## ダウンロードの流れ（`cfs sync`）

1. `location` が 32 桁 hex ならバケットハッシュ、そうでなければタグ名として `tag/<name>` を GET してハッシュを得る
2. バケット本体を `Fetch`（`DefaultContentAttribute()` でデコード）→ TSV をパース
3. 必要なら `--filter-cmd` でファイルを絞り込む
4. 各 `Content` を `Fetch`。`~/.cfs/datacache/<hash>` にあればそれを使い、なければダウンロードしてキャッシュ（キャッシュはエンコード済みのまま保存）→ デコードして出力先に atomic write

更新がなければ、タグ（32 バイトのハッシュ）とバケットの取得だけで終わる、というのが速度面の売り。

## 関連記事

- [storage-layout-and-bucket-format.md](storage-layout-and-bucket-format.md) — キャビネット上の配置とバケット形式、暗号化/圧縮
- [cli-and-configuration.md](cli-and-configuration.md) — コマンドと設定ファイル
- [pack-format.md](pack-format.md) — パックファイルとパッチ
- [development-build-and-ci.md](development-build-and-ci.md) — ビルド・テスト・リリース
- [known-issues-and-gotchas.md](known-issues-and-gotchas.md) — 落とし穴と既知の不具合
