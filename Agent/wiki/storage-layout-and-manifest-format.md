# キャビネットの構成とマニフェスト形式（実装メモ）

**形式の仕様そのものは人間向けの `docs/format.md` が正。** ここには実装の対応関係と、仕様に書かない勘所だけを書く。形式を変えたら `docs/format.md` を先に直す。

## 形式 v2（2026-10-25〜）の要点

- ハッシュは SHA-256（`util.go` の `SumHash`、`HashLength = 64`）。`isHash` は 64 文字の小文字 16 進だけを受け付ける
- 配置: `data/` 中身、`manifest/` マニフェスト、`tag/` タグ、`config.json` キャビネット設定、`gc/` GC の状態（`storage.go` の先頭コメント）。場所の組み立ては `dataKey` / `manifestKey` / `tagKey`
- マニフェストは `#cfs-manifest 2` で始まる TSV（`manifest.go` の `Dump` / `Parse`）。署名は最後の `#signature ed25519 <鍵 ID> <署名>` 行
- 暗号化は `crypt.go`。`EncryptKey` から HMAC-SHA256 で `enc_key` / `iv_key` / `key_id` を導出。暗号文は `[版 1B][鍵 ID 8B][IV 16B][AES-256-CTR]`。IV は `HMAC(iv_key, 平文)` の先頭 16 バイト（重複の排除のため中身から決める）
- 署名は `signing.go`（Ed25519）。秘密鍵は環境変数 `CFS_SIGNING_KEY` だけから読む（`OptionInfo.SigningKey` は `json:"-"`）。公開鍵は `.cfsenv` の `VerifyKeys`
- `Downloader.fetchObject` は取得したバイト列の SHA-256 を必ず確かめる（手元のキャッシュが壊れていたら取り直す）
- v1 との互換性はない（オーナー指示で互換性は当面不要）

## コードの対応

| 役割 | 場所 |
|---|---|
| キャビネットへの書き込み・一覧・削除 | `Storage`（`Put` / `Exists` / `List` / `Delete`）: `FileStorage`、`S3Storage`、`DummyStorage`（テスト用） |
| キャビネットからの読み込み | `Fetcher`（`Open` / `Exists`）: `fileFetcher`、`httpFetcher`、`S3Storage`、`DummyStorage` |
| アップロード | `Client`（`Init` で鍵と署名鍵を読む。`Upload` → 0 バイト以外を `data/` に Put。`UploadManifest` → `manifest/` に Put、手元キャッシュと `.hash`、タグ） |
| 手元キャッシュの信用 | `Manifest.ValidateCache`（前回のマニフェストが `manifest/` にあるか 1 回確かめる。`cmd/cfs/main.go` の `doUpload` から呼ぶ） |
| ダウンロード | `Downloader`（`ResolveLocation` → `LoadManifest` → `Fetch`） |
| GC・タグ・キャビネット設定 | `gc.go`（`RunGC`、`ListTags`、`DeleteTags`、`LoadCabinetConfig` / `SaveCabinetConfig`、`ValidateTagName`） |
| 形式の版 | `config.json` の `format`（`CabinetFormat = 2`）。`CabinetConfig.CheckFormat` が、0（書かれていない）か 2 以外を拒否する。`Downloader.LoadManifest`（最初の 1 回だけ）、`doUpload`、`RunGC`、`SaveCabinetConfig` で確かめる |
| キャビネットの作成・IAM ポリシー | `cabinet_init.go`（`InitCabinet`、`initS3Cabinet`、`IAMPolicy`）。S3 は CreateBucket → PutPublicAccessBlock（4 項目すべて true）→ PutBucketOwnershipControls（`BucketOwnerEnforced`）→ `config.json`。`BucketAlreadyOwnedByYou` は続行、`BucketAlreadyExists` はエラー。dry-run は `dryRunFetcher` で何も読まない |

## バックエンドごとの実装

| スキーム | 書き込み | 読み込み | 認証 |
|---|---|---|---|
| `file:///path` やパス | `FileStorage`（`atomic.WriteFile`） | `fileFetcher`（ファイルを直接読む） | なし |
| `s3://<bucket>` | `S3Storage`（PutObject、ACL `bucket-owner-full-control`、既存確認に ListObjectsV2、DeleteObjects は 1000 件ずつ） | `S3Storage`（GetObject / HeadObject） | SDK 標準の認証チェーン |
| `http(s)://` | なし（読み込み専用） | `httpFetcher` | なし |

`gs://` は 2026-10-25 に外した（`StorageFromUrl` が「未対応」のエラーを返す）。

## 圧縮・暗号化の属性

- ファイルごとの属性は `Client.attribute` で決まる（既定: `Compress` なら圧縮、`EncryptKey` があれば暗号化）
- 拡張子 `.ab` `.raw` `.pbx` `.mp4` は **常に無圧縮・無暗号**（AssetBundle はクライアントでの復号コストが高いため意図的に無暗号、とオーナーに確認済み 2026-10-25）。コード上にハードコード
- マニフェスト自体はアップロード側の設定の既定の属性（`Config.DefaultAttribute()`）で置く。読む側（Downloader と GC）は `decodeManifest` で 4 通りの属性を試して読むので、`Compress` の設定は一致していなくてよい
