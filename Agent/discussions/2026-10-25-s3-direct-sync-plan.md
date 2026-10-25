# S3 から HTTP 公開なしで sync する計画（検討中）

**状態: 方針の案まで。オーナーが他にも相談したい点があるため、実装は未着手。**

## 背景

現在のダウンロード側（`Downloader`）は、`Url`（未指定なら `Storage.DownloaderUrl()`）に対して HTTP GET / HEAD するだけの作り。S3 の場合は静的ウェブサイトホスティングで公開読み取りにしておく必要がある。バケットを公開せずに、認証付きで直接 `sync` などをしたい。

## 方針案

`Downloader` から「パスを指定してバイト列を取ってくる部分」だけをインターフェースに切り出し、URL スキームで実装を選ぶ。

```go
// Downloader が使う読み込み口（キャッシュ・復号・タグ解決は今の Downloader のまま）
type Fetcher interface {
	Fetch(path string) ([]byte, error)  // "tag/v1", "data/c5/9548..." などを取得
	Exists(path string) (bool, error)   // ls --verify 用
}
```

- HTTP 実装: 現在の `fetch` / `headRequest` をそのまま移す（`http://` `https://` `file://`）
- S3 実装: aws-sdk-go-v2 の GetObject / HeadObject。クライアント生成は `S3Storage` と共通化
- `NewDownloader` が `Url` のスキームで実装を選ぶ

設定は `.cfsenv` の `Url` に `s3://<bucket>` を書いたときだけ S3 直接取得にする。

```json
{ "Cabinet": "s3://my-bucket", "Url": "s3://my-bucket" }
```

- `Url` 未指定時の挙動は変えない（S3 静的サイトの http URL を使う）。既存の公開運用を壊さないため
- コードは読んでいるのに定義されていない `--url` グローバルフラグを正式に定義し、一時的な切り替えに使えるようにする
- 認証はアップロードと同じ SDK 標準チェーン（環境変数・`~/.aws`・SSO・IAM ロール）

### この形にする理由

- `sync` / `ls`（`--verify`）/ `cat` / `merge` / `pack-bucket` / `http` はすべて `NewDownloader` を通るので、まとめて対応できる
- キャッシュ（`~/.cfs/datacache`）、復号、0 バイトファイルの扱い、リトライには手を入れない
- HTTP 側の挙動は `Agent/scripts/behavior-golden/` で変化なしを確認できる。S3 側は既存の S3 偽サーバーテストに GET / HEAD を足して確認する

### 採らなかった案

- `Storage` インターフェースに読み込みを足す: File / GCS / Dummy すべてに実装が要り、書き込みと読み込みの責務が混ざる
- 署名付き URL を発行して HTTP で取る: ファイルごとの署名が要り、URL が変わるのでキャッシュも効きにくい。認証情報を持たせられない配布先（ゲームクライアントなど）向けの手段で、手元からの sync には過剰

## 注意点・未決事項

- 必要な権限は `s3:GetObject`。`s3:ListBucket` がないと、存在しないオブジェクトへの HeadObject が 404 ではなく 403 になるので、`Exists` では 403 も「存在しない」扱いにする必要がある
- リージョンは `ap-northeast-1` 固定のまま（設定可能にするかは別途）
- `s3://bucket/prefix` はアップロード側も含めて prefix を無視しているので今回は対象外
- `Downloader.Sync` は逐次実行。S3 はリクエストあたりの待ち時間が HTTP より長めなので、遅ければ並列化を次の改善とする
- 同じ形で `gs://` の実装も追加できる
- オーナーの追加の相談事項を受けてから確定する
