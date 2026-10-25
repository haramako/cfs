# アーキテクチャ概要

CFS (Content base File System) は、ゲームアセットのような大量ファイルを **内容ハッシュ (SHA-256。2026-10-25 までは MD5) 単位で差分アップロード／ダウンロード** する Go 製ツール。git のオブジェクトストアに近い構造で、同一内容のファイルはバージョンやプラットフォームをまたいで 1 つに共有される。

- モジュール: `github.com/haramako/cfs`（`go.mod` は go 1.26）
- 成果物: CLI `cfs`（`cmd/cfs`）のみ。実装は `internal/` にあり、外部からライブラリとしては使えない（2026-10 に CLI 専用と決定）
- 開発期間: 2015-09 開始、2021-06 以降は休眠、2026-10 に構成と依存を現代化

## パッケージ構成

`internal/cfs/` がコア実装（package `cfs`）、`internal/pack/` がパック形式、`cmd/cfs/` が CLI。

| パス（`internal/cfs/` 内） | 役割 |
|---|---|
| `manifest.go` | `Manifest`（ファイル一覧。2026-10-25 に Bucket から改名）と `Content`（1 ファイル分のメタ）、`ContentAttribute`（圧縮/暗号フラグ）、マニフェストの TSV シリアライズ |
| `client.go` | アップロード側。ディレクトリ走査 → 変更検出 → エンコード → ワーカーで並列アップロード → マニフェスト/タグのアップロード |
| `downloader.go` | ダウンロード側。タグ解決 → マニフェスト取得 → データ取得（ローカルキャッシュ経由）→ デコード |
| `storage.go` | `Storage` インターフェースと URL スキームからの生成（`file://` / `s3://`。`gs://` は 2026-10-25 に外した） |
| `file_storage.go` / `s3_storage.go` | 各バックエンドの書き込み実装 |
| `dummy_storage.go` | テスト用のメモリ内ストレージ |
| `util.go` | ハッシュ（SHA-256）、キャビネット上の場所（`dataKey` など）、zlib 圧縮と暗号化の `encode`/`decode` |
| `crypt.go` / `signing.go` | 暗号化（AES-256-CTR、鍵の導出、鍵 ID）とマニフェストの署名（Ed25519） |
| `gc.go` | GC、タグの一覧・削除、キャビネットの設定（`config.json`） |
| `fetcher.go` / `location.go` | 読み込み口（ファイル・HTTP）と、URL・パスの判定 |
| `config.go` | 設定 `Config`（`.cfsenv` の JSON と環境変数）、キャッシュの場所、`Logger`。グローバル変数ではなく引数で渡す |
| `internal/pack/` | 独自バイナリ形式のパックファイル（`TP` 形式）。詳細は [pack-format.md](pack-format.md) |
| `cmd/cfs/` | CLI（urfave/cli v3）。コマンド一覧は [cli-and-configuration.md](cli-and-configuration.md) |

主な外部依存: S3 は aws-sdk-go-v2、atomic な書き込みに natefinch/atomic、並列処理に golang.org/x/sync/errgroup。

## 書き込みと読み込みの非対称性

**書き込み（アップロード）は `Storage` 実装経由、読み込み（ダウンロード）は `Fetcher` 経由**という設計になっている（2026-10-25 に `Fetcher` を切り出した）。

- `Storage` インターフェースは `Upload` / `UploadTag` だけ（書き込み専用）。読み込みは `Fetcher`（`Open` / `Exists`）。`S3Storage` は両方を実装する
- `Downloader` は `Fetcher`（`internal/cfs/fetcher.go`）で `data/xx/yyyy…` と `tag/<name>` を読む。実装は `httpFetcher`（http/https）と `fileFetcher`（`file://` と手元のパス。直接ファイルを読む）。ないときは `ErrNotFound` を包んだエラー
- ダウンロード元は `.cfsenv` の `Url`（または `--url`）。**未指定ならキャビネット自体から直接読む**（`cmd/cfs/main.go` の `getDownloaderURL` が `Cabinet` を返す。2026-10-25 から。それ以前は S3 静的サイトの http URL を使っていた）
- S3 のバケットは非公開でよい。認証情報を持たせられない配布先には、CDN などで公開して `Url` に http(s) の URL を書く

## アップロードの流れ（`cfs upload`）

1. `.cfsenv` を読み、ローカルのマニフェストのキャッシュ（前回アップロード時のマニフェスト）を 手元のキャッシュの `manifest/<sha256>` からロード。`.hash` のハッシュで `manifest/` にまだあるかを 1 回確かめ（`ValidateCache`）、なければキャッシュを捨てる
2. `Client.AddFiles` で対象ディレクトリを走査（除外ルールは [known-issues-and-gotchas.md](known-issues-and-gotchas.md) 参照）
3. `Client.AddFile` で変更検出
   - 前回の `Time + 1秒` 以前の mtime かつサイズ一致 → 読まずにスキップ
   - 読み込んで SHA-256（`OrigHash`）が一致 → 時刻だけ更新
   - それ以外 → `encode`（圧縮→暗号化）してキューに積む
4. 32 並列の `uploadWorker` が `Storage.Put(data/..., overwrite=false)` を呼ぶ（既にあればバックエンド側でスキップ。0 バイトのデータも置く）
5. `RemoveUntouched` で今回見つからなかったエントリを削除（マニフェスト＝ディレクトリの完全なスナップショット）
6. `Client.Finish`: アップロードの完了を待ち、失敗したものがあればマニフェストを置かずにエラーを返す。成功なら `UploadManifest`: マニフェスト（署名の鍵があれば署名付き）をエンコードして `manifest/` に置き、そのハッシュをタグ `tag/<name>` に書く。ローカルキャッシュと `<cache>.hash` も更新

## ダウンロードの流れ（`cfs sync`）

1. `location` が 64 桁 hex ならマニフェストハッシュ、そうでなければタグ名として `tag/<name>` を GET してハッシュを得る
2. マニフェスト本体を取得し、`decodeManifest` で圧縮・暗号化をデータから判断して戻す → TSV をパース（`VerifyKeys` があれば署名を確かめる）
3. 必要なら `--filter-cmd` でファイルを絞り込む
4. 各 `Content` を 8 並列で `Fetch`（失敗したら 3 回までやり直す）。手元のキャッシュの `data/<hash>` にあればそれを使い、なければダウンロードしてキャッシュ（キャッシュはエンコード済みのまま保存）→ デコードして出力先に atomic write

更新がなければ、タグ（32 バイトのハッシュ）とマニフェストの取得だけで終わる、というのが速度面の売り。

## 関連記事

- [storage-layout-and-manifest-format.md](storage-layout-and-manifest-format.md) — キャビネット上の配置とマニフェスト形式、暗号化/圧縮
- [cli-and-configuration.md](cli-and-configuration.md) — コマンドと設定ファイル
- [pack-format.md](pack-format.md) — パックファイルとパッチ
- [development-build-and-ci.md](development-build-and-ci.md) — ビルド・テスト・リリース
- [known-issues-and-gotchas.md](known-issues-and-gotchas.md) — 落とし穴と既知の不具合
