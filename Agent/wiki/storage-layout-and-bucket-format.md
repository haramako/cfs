# キャビネットの構成とバケット形式

## 用語

| 用語 | 意味 |
|---|---|
| キャビネット (cabinet) | アップロード先のストレージ全体。`.cfsenv` の `Cabinet` に URL で指定 |
| バケット (bucket) | 1 回のアップロードで得られるファイル一覧（マニフェスト）。GCS/S3 の「バケット」とは別物なので注意 |
| タグ (tag) | バケットハッシュへの名前付きポインタ（git のブランチ/タグに相当） |
| location | CLI で「どのバケットか」を指す引数。タグ名またはバケットハッシュ（32 桁 hex） |

## キャビネット上の配置

```
<cabinet>/
  data/
    c5/9548c3c576228486a1f0037eb16a1b   # コンテンツデータ（ハッシュの先頭2文字/残り30文字）
    ...                                 # バケット自体もここに置かれる
  tag/
    <tag名>                             # 中身はバケットハッシュ 32 文字のプレーンテキスト
```

- `data/` 以下は **内容アドレス**: 同じパスの中身は二度と変わらないので、HTTP プロキシや CDN で安全にキャッシュできる
- `tag/` 以下は上書きされるのでキャッシュしてはいけない
- 使われなくなった `data/` を消すガーベージコレクトは未実装（README の TODO に残っている）

### バックエンドごとの URL

| スキーム | 書き込み | `DownloaderUrl()` | 認証 |
|---|---|---|---|
| `file:///path` | ローカルファイル書き込み | `file:///path/` | なし |
| `gs://<bucket>` | cloud.google.com/go/storage（`DoesNotExist` 条件で新規作成のみ） | `http://storage.googleapis.com/<bucket>/` | Application Default Credentials（`STORAGE_EMULATOR_HOST` でエミュレーター可） |
| `s3://<bucket>` | aws-sdk-go-v2 で PutObject（ACL `bucket-owner-full-control`、既存チェックに ListObjectsV2） | `http://<bucket>.s3-website-ap-northeast-1.amazonaws.com/` | SDK 標準の認証チェーン（環境変数・共有設定・SSO・IAM ロール） |

S3 のリージョンは `ap-northeast-1` 固定（`AWS_REGION` より優先）。チェックサムは「必要な場合のみ」に設定して、旧実装と同じ素の PutObject にしている。認証情報は実際のリクエスト時まで検証されない（旧 goamz 実装は環境変数がないと `NewS3Storage` の時点でエラーだった）。`cfs://`（旧サーバー `cfssv`）は 2019-08 に削除済みで、`StorageFromUrl` も対応していない。

## ハッシュ

- アルゴリズムは MD5（`Bucket.HashType = "md5"`、`sha1` は未対応で panic）
- `Hash`: **ストレージに置かれるバイト列（圧縮・暗号化後）** の MD5。`data/` のパスはこちら
- `OrigHash`: 元ファイルの MD5。変更検出、`ls` 表示、パックファイルに使う
- 圧縮も暗号化もしないファイルは `Hash == OrigHash`

## バケットファイル形式

改行区切りの TSV。各行 7 列、パス順にソート（`Bucket.Dump`）。

| 列 | 内容 |
|---|---|
| 0 | `Hash`（格納データの MD5） |
| 1 | `Path`（`/` 区切りの相対パス。`Flatten` 時はファイル名のみ） |
| 2 | `Size`（格納データのバイト数） |
| 3 | `Time`（mtime、RFC3339） |
| 4 | `OrigHash` |
| 5 | `OrigSize` |
| 6 | `Attr`（`ContentAttribute`: bit0=圧縮, bit1=暗号） |

バケット自体も `DefaultContentAttribute()` でエンコードして `data/` に格納される。ローカルキャッシュ（`~/.cfs/cache/…`）にはエンコード前の TSV が書かれる。

## 暗号化と圧縮

- エンコード順: **zlib 圧縮 → AES-CFB 暗号化**。デコードはその逆（`util.go`）
- 鍵・IV は `.cfsenv` の `EncryptKey`（16/24/32 バイト）と `EncryptIv`（16 バイト）。`EncryptKey` が空なら暗号化しない
- ファイルごとの属性は `Bucket.GetAttribute` で決まる
  - 既定: `Compress` が true なら圧縮、`EncryptKey` があれば暗号化
  - 拡張子 `.ab` `.raw` `.pbx` `.mp4` は **常に無圧縮・無暗号**（Unity AssetBundle や動画など、容量が大きく既に圧縮済みのもの向け。AssetBundle はクライアントでの復号コストが高いため意図的に無暗号、とオーナーに確認済み 2026-10-25）。コード上にハードコードされており、README にある「`.cfsenv` で指定」は未実装
- 属性はバケットの各行に保存されるので、ダウンロード側はファイルごとに正しくデコードできる。ただし **バケット自体のデコードはダウンロード側の `Option`（`DefaultContentAttribute()`）に依存** するため、アップロード側と同じ `Compress`/`EncryptKey` 設定が必要
