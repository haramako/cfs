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
| `bucket.go` | ファイル一覧（バケット）と、その保存形式 |
| `client.go` | アップロード（変更の検出、並列アップロード） |
| `downloader.go` | ダウンロード（タグの解決、キャッシュ、並列取得） |
| `storage.go` | ストレージの共通の形と、URL からの選択 |
| `file_storage.go` / `gcs_storage.go` / `s3_storage.go` | 各ストレージへの書き込み |
| `util.go` | 圧縮と暗号化 |
| `option.go` / `config.go` | `.cfsenv` と `~/.cfs_setting` の読み込み |

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

通常のテストは、一時フォルダを使ったローカルのストレージで動きます。S3 と GCS のテストは、偽の HTTP サーバーを相手にするので、認証情報は不要です。

本物の S3 や GCS を相手にテストすることもできます（認証情報が必要です）。

```
CFS_TEST_STORAGE=gcs go test ./internal/cfs     # gs://cfs を使います
CFS_TEST_STORAGE=s3  go test ./internal/cfs     # s3://cfs-autotest を使います
```

`CFS_TEST_VERBOSE=1` を付けると、詳しいログを表示します。

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

goreleaser が windows、darwin、linux（amd64）の実行ファイルを zip にまとめ、GitHub Releases に置きます。設定は `.goreleaser.yml` にあります。

## ドキュメント

- 使う人向けの説明は `docs/` に書きます。コマンドや設定の挙動を変えたら、合わせて更新してください
- `Agent/` は AI エージェントが作業のために使うメモです。人間向けの説明は `docs/` を正としてください
