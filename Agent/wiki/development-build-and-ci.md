# ビルド・テスト・リリース

## 構成

- 標準的な Go モジュール構成: `cmd/cfs/`（main）、`internal/cfs/`、`internal/pack/`。ルートに Go パッケージはない
- `go.mod` は `go 1.26.0`。import パスは `github.com/haramako/cfs/internal/...`（旧来の `local.package/cfs` + `replace` は 2026-10 に廃止）
- `go install github.com/haramako/cfs/cmd/cfs@latest` でもインストールできる

## ビルド

```
make            # darwin / linux / windows (amd64) を bin/<os>/ に出力
make windows    # 個別
go build ./cmd/cfs
```

- `-ldflags "-X main.revision=<git short hash>"` でリビジョンを埋め込み、`cfs -v` で `cfs version 0.0.0 <rev>` と表示される。バージョン番号部分は固定の `0.0.0`
- Windows の開発機には `make` がないことがある。その場合は `go build` を直接使う

## テスト

```
go test ./...                                   # file:// ストレージ（一時ディレクトリ）でテスト
CFS_TEST_STORAGE=s3  go test ./internal/cfs     # 実 S3 の s3://cfs-autotest を使う（要 AWS 認証）
CFS_TEST_VERBOSE=1   go test ./internal/cfs     # 詳細ログ
```

- `manifest_test.go` が主要な結合テスト（新規アップロード、上書き、圧縮、無圧縮拡張子、タグ）。`TestMain` で暗号鍵/IV を固定値に設定している
- `s3_storage_test.go` は `httptest` の偽サーバーに対して、送られるリクエスト（パス・ACL・Content-Type・既存確認・0 バイト時の挙動）を確認する。`newS3Storage` の `optFns` でエンドポイントを差し替えている。GCS の偽サーバーテストは GCS と一緒に外した（git 履歴の 20d4f1d に残る）
- `client_test.go` は `DummyStorage` を使ったアップロードテスト（失敗注入部分はコメントアウトされたまま）
- `internal/pack/pack_test.go` はパック/パースの往復テスト
- `config_test.go` は実際のホームディレクトリに `~/.cfs/` を作る

### CLI の挙動比較（ゴールデンテスト）

`Agent/scripts/behavior-golden/golden.sh` で、2 つの `cfs` バイナリの外部から見える挙動（upload/ls/cat/merge/sync/pack/unpack/patch/http の出力、キャビネットとキャッシュの中身）を比較できる。リファクタリング前後で diff を取る用途。詳細は同フォルダの README。

## CI / リリース（GitHub Actions）

- `.github/workflows/ci.yml`: push / PR で ubuntu と windows の `go vet ./...` と `go test -v ./...`
- `.github/workflows/release.yml`: `v*` タグで `go test` → goreleaser（`goreleaser-action@v7`, goreleaser v2）
- `.goreleaser.yml` は `version: 2` 形式。3 OS × amd64 の zip（README 同梱）とチェックサムを GitHub Releases に出す。`main.revision` に短いコミットハッシュを埋め込む。`release.prerelease: auto` なので `-` 付きのタグ（`v0.0.0-test.1` など）はプレリリースになり、Latest を奪わない
- 2026-10-25 に `v0.0.0-test.1` で動作確認済み（成功、約3分。zip は各約 13MB。GCS の依存を外せば小さくなるはず）。確認用のリリースとタグは削除済み。詳細は `../discussions/2026-10-25-release-workflow-check.md`
- 旧 CircleCI（Go 1.14 イメージ、終了済みの `git.io` に依存）は 2026-10 に削除

## 歴史メモ

- 2015-09: 開始
- 2019-08: サーバー実装（`cfssv`, `cfs://`）を削除し、pack/unpack/pack-manifest を追加
- 2019-09〜11: `merge --filter-cmd`、mtime 1 秒誤差の許容
- 2020-08: `http` サーバーのポート指定
- 2020-12: CircleCI 導入、フェッチのリトライ、`~/.cfs_setting` の `HomeRoot`
- 2021-01〜03: `patch`、`sync --filter-cmd`、`ls --verify`（並列化）、0 バイトファイル対応、`DummyStorage`
- 2021-06: リビジョン埋め込み
- 2026-10: `internal/` への移動、Go 1.26 化、GitHub Actions + goreleaser v2、依存更新（aws-sdk-go-v2 / cloud.google.com/go/storage / urfave/cli v3 / yaml v3）
