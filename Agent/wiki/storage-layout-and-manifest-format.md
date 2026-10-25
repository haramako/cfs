# キャビネットの構成とマニフェスト形式

## 用語

| 用語 | 意味 |
|---|---|
| キャビネット (cabinet) | アップロード先のストレージ全体。`.cfsenv` の `Cabinet` に URL で指定 |
| マニフェスト (manifest) | 1 回のアップロードで得られるファイル一覧。2026-10-25 に「バケット」から改名（S3 のバケットとまぎらわしいため） |
| タグ (tag) | マニフェストハッシュへの名前付きポインタ（git のブランチ/タグに相当） |
| location | CLI で「どのマニフェストか」を指す引数。タグ名またはマニフェストハッシュ（32 桁 hex） |

## キャビネット上の配置

```
<cabinet>/
  data/
    c5/9548c3c576228486a1f0037eb16a1b   # コンテンツデータ（ハッシュの先頭2文字/残り30文字）
    ...                                 # マニフェスト自体もここに置かれる
  tag/
    <tag名>                             # 中身はマニフェストハッシュ 32 文字のプレーンテキスト
```

- `data/` 以下は **内容アドレス**: 同じパスの中身は二度と変わらないので、HTTP プロキシや CDN で安全にキャッシュできる
- `tag/` 以下は上書きされるのでキャッシュしてはいけない
- 使われなくなった `data/` を消すガーベージコレクトは未実装（README の TODO に残っている）

### バックエンドごとの URL

| スキーム | 書き込み | 読み込み | 認証 |
|---|---|---|---|
| `file:///path` やパス | ローカルファイル書き込み | ファイルを直接読む（`fileFetcher`） | なし |
| `s3://<bucket>` | aws-sdk-go-v2 で PutObject（ACL `bucket-owner-full-control`、既存チェックに ListObjectsV2） | GetObject / HeadObject（`S3Storage` が `Fetcher` も実装） | SDK 標準の認証チェーン（環境変数・共有設定・SSO・IAM ロール） |

S3 のリージョンは自動判定（詳細は known-issues-and-gotchas.md の S3 の項）。チェックサムは「必要な場合のみ」に設定して、旧実装と同じ素の PutObject にしている。認証情報は実際のリクエスト時まで検証されない（旧 goamz 実装は環境変数がないと `NewS3Storage` の時点でエラーだった）。`cfs://`（旧サーバー `cfssv`）は 2019-08 に削除済みで、`StorageFromUrl` も対応していない。

## ハッシュ

- アルゴリズムは MD5（`Manifest.HashType = "md5"`、`sha1` は未対応で panic）
- `Hash`: **ストレージに置かれるバイト列（圧縮・暗号化後）** の MD5。`data/` のパスはこちら
- `OrigHash`: 元ファイルの MD5。変更検出、`ls` 表示、パックファイルに使う
- 圧縮も暗号化もしないファイルは `Hash == OrigHash`

## マニフェストの形式

改行区切りの TSV。各行 7 列、パス順にソート（`Manifest.Dump`）。

| 列 | 内容 |
|---|---|
| 0 | `Hash`（格納データの MD5） |
| 1 | `Path`（`/` 区切りの相対パス。`Flatten` 時はファイル名のみ） |
| 2 | `Size`（格納データのバイト数） |
| 3 | `Time`（mtime、RFC3339） |
| 4 | `OrigHash` |
| 5 | `OrigSize` |
| 6 | `Attr`（`ContentAttribute`: bit0=圧縮, bit1=暗号） |

マニフェスト自体も `DefaultContentAttribute()` でエンコードして `data/` に格納される。ローカルキャッシュ（`~/.cfs/cache/…`）にはエンコード前の TSV が書かれる。

## 暗号化と圧縮

- エンコード順: **zlib 圧縮 → AES-CFB 暗号化**。デコードはその逆（`util.go`）
- 鍵・IV は `.cfsenv` の `EncryptKey`（16/24/32 バイト）と `EncryptIv`（16 バイト）。`EncryptKey` が空なら暗号化しない
- ファイルごとの属性は `Manifest.GetAttribute` で決まる
  - 既定: `Compress` が true なら圧縮、`EncryptKey` があれば暗号化
  - 拡張子 `.ab` `.raw` `.pbx` `.mp4` は **常に無圧縮・無暗号**（Unity AssetBundle や動画など、容量が大きく既に圧縮済みのもの向け。AssetBundle はクライアントでの復号コストが高いため意図的に無暗号、とオーナーに確認済み 2026-10-25）。コード上にハードコードされており、README にある「`.cfsenv` で指定」は未実装
- 属性はマニフェストの各行に保存されるので、ダウンロード側はファイルごとに正しくデコードできる。ただし **マニフェスト自体のデコードはダウンロード側の `Option`（`DefaultContentAttribute()`）に依存** するため、アップロード側と同じ `Compress`/`EncryptKey` 設定が必要
