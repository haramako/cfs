# 開発

## 必要なもの

- Go 1.26 以上

## フォルダ構成

```
cmd/cfs/          コマンド（main パッケージ）
internal/cfs/     本体（アップロード、ダウンロード、ストレージ、設定）
internal/pack/    パックファイル
docs/             人間向けのドキュメント
Agent/            AI エージェント向けの作業メモ（wiki、経緯ログ、補助スクリプト）
```

本体は `internal/` にあるので、外部から Go のライブラリとして使うことはできません。CFS はコマンドとして使う前提です。

### `internal/cfs/` の主なファイル

| ファイル | 内容 |
|---|---|
| `manifest.go` | ファイル一覧（マニフェスト）と、その保存形式 |
| `client.go` | アップロード（変更の検出、並列アップロード） |
| `downloader.go` | ダウンロード（タグの解決、キャッシュ、並列取得） |
| `storage.go` | ストレージ（書き込み・一覧・削除）の共通の形と、URL からの選択 |
| `fetcher.go` | ダウンロード元から読む口（ファイル、HTTP）|
| `location.go` | URL と手元のパスの判定・変換 |
| `file_storage.go` / `s3_storage.go` | 各ストレージ（S3 は読み込みとリージョンの判定も） |
| `crypt.go` / `signing.go` | 暗号化（鍵の導出、鍵 ID）と、マニフェストの署名 |
| `gc.go` | GC、タグの一覧と削除、キャビネットの設定 |
| `util.go` | ハッシュ、キャビネット上の場所、圧縮と暗号化の組み合わせ |
| `config.go` | 設定（`Config`）。`.cfsenv` と環境変数の読み込み、キャッシュの場所。設定はグローバル変数ではなく、`Client` や `Downloader` などに引数で渡します |

## ビルド

```
go build ./cmd/cfs
```

3つの OS 向けにまとめてビルドする場合は `make` を使います（`bin/<OS>/` に出力します）。

```
make            # darwin / linux / windows（amd64）
make windows    # 1つだけ
```

`make` はビルドに git のコミットのハッシュを埋め込みます。`cfs -v` で確認できます。

## テスト

```
go test ./...
go vet ./...
```

通常のテストは、一時フォルダを使ったローカルのストレージで動きます。S3 のテストは、偽の HTTP サーバーを相手にするので、認証情報は不要です。

`cmd/cfs/cli_test.go` は、CLI のコマンド（upload / sync / ls / cat / merge / tag / gc / cabinet / keygen / pack / unpack / patch / pack-manifest / `--filter-cmd`）を、一時フォルダのキャビネットに対してプロセスの中で実行して確かめます。

カバレッジは次のように測れます。

```
go test -coverpkg=./... -coverprofile=cover.out ./...
go tool cover -func=cover.out
```

`CFS_TEST_VERBOSE=1` を付けると、詳しいログを表示します。

### 本物の S3 でのテスト

本物の S3 のバケットを相手にテストすることもできます。テストは中身を頻繁に書き込み・削除するので、**ふだんの認証情報ではなく、テスト用のバケットだけを操作できる IAM ロール**で動かします。

**準備（最初の 1 回だけ）**

1. テスト用のバケットを作ります

   ```
   cfs -c s3://<テスト用バケット> cabinet init
   ```

2. テスト用のバケットだけを操作できる IAM ロールを作ります。権限は `cfs -c s3://<テスト用バケット> cabinet policy gc` の出力を、ロールのインラインポリシーにします。信頼ポリシーは、自分の IAM ユーザー（またはロール）からの `sts:AssumeRole` だけを許可します
3. `~/.aws/config` に、そのロールに切り替えるプロファイルを足します

   ```
   [profile cfs-autotest]
   role_arn = arn:aws:iam::<アカウント>:role/cfs-autotest
   source_profile = default
   region = <リージョン>
   ```

4. リポジトリの直下に `.cfs-test.env` を作ります（`.gitignore` 済みなので、リポジトリには入りません）

   ```
   CFS_TEST_STORAGE=s3
   CFS_TEST_S3_BUCKET=<テスト用バケット>
   AWS_REGION=<リージョン>
   AWS_PROFILE=cfs-autotest
   ```

**実行**

```
scripts/with-test-aws.sh go test -count=1 ./internal/cfs
```

`scripts/with-test-aws.sh` は、`.cfs-test.env` を読み、AWS CLI でロールの一時的な認証情報を取り出して、そのコマンドにだけ環境変数として渡します。`cfs` のコマンドも同じように実行できます（例: `scripts/with-test-aws.sh cfs -c s3://<テスト用バケット> tag ls`）。

AWS CLI を経由しているのは、`aws login` でログインしたプロファイルを `source_profile` にしてロールを引き受けることに、Go の AWS SDK（aws-sdk-go-v2）がまだ対応していないためです（AWS CLI は対応しています）。

### コマンドの挙動を比べる

`Agent/scripts/behavior-golden/golden.sh` で、2つの `cfs` の実行ファイルの挙動を比べられます。アップロード、ダウンロード、パックファイル、HTTP サーバーの出力と、保存されたファイルの中身を記録します。リファクタリングの前後で挙動が変わっていないかを確かめるときに使います。使い方は同じフォルダの README を見てください。

## CI

GitHub Actions で動きます。

- `.github/workflows/ci.yml`: push と pull request のたびに、ubuntu と windows で `go vet` と `go test` を実行します
- `.github/workflows/release.yml`: `v` で始まるタグを push すると、テストの後に goreleaser でリリースを作ります

## リリース

```
git tag v0.2.0
git push origin v0.2.0
```

goreleaser が windows、darwin、linux（amd64）の実行ファイルを zip にまとめ、チェックサムのファイルと一緒に GitHub Releases に置きます。設定は `.goreleaser.yml` にあります。リリースの完成まで、数分かかります。

`v0.2.0-rc.1` のように `-` の付いたバージョンは、GitHub でも**プレリリース**として公開されます。プレリリースは「Latest」にならないので、リリースの手順を試すときはこの形のタグを使ってください。

## ドキュメント

- 使う人向けの説明は `docs/` に書きます。コマンドや設定の挙動を変えたら、合わせて更新してください
- `Agent/` は AI エージェントが作業のために使うメモです。人間向けの説明は `docs/` を正としてください
