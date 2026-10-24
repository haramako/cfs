# behavior-golden

## 目的

`cfs` バイナリの外部から見える挙動を 1 つのテキストに記録し、2 つのバイナリ（例: リファクタリング前後）の差分を取る。2026-10 の構成・依存の現代化で、挙動が変わっていないことの確認に使った。

## 実行方法

```bash
go build -ldflags "-X main.revision=x" -o /tmp/cfs-old.exe ./cmd/cfs   # 比較元（git stash / worktree などで旧版をビルド）
go build -ldflags "-X main.revision=x" -o /tmp/cfs-new.exe ./cmd/cfs
bash Agent/scripts/behavior-golden/golden.sh /tmp/cfs-old.exe /tmp/old.txt /tmp/cfs-golden-work
bash Agent/scripts/behavior-golden/golden.sh /tmp/cfs-new.exe /tmp/new.txt /tmp/cfs-golden-work
diff /tmp/old.txt /tmp/new.txt
```

- 第 3 引数の作業ディレクトリは **両方で同じパス** にする（パスがバケットキャッシュ名や出力に含まれるため）
- revision も両方で同じ値にする（`-h` の出力に入る）

## 前提

- Git Bash（`cygpath`, `touch -d`, `md5sum`, `curl`, `grep`）と Go
- ポート 18764（キャビネット配信用の `static-server/`）と 18765（`cfs http`）が空いていること
- ホームディレクトリは作業ディレクトリ内の `home/` に差し替えるので、実際の `~/.cfs` は汚さない

## 記録する内容

- `-h` / `help <cmd>` / `config` / `setting`
- `upload`（初回・変更なし・変更あり・`-f`・引数の後ろのフラグ）、`ls`（タグ・ハッシュ・`--verify`）、`cat`、`merge`、`sync`（`--filter-cmd` 含む）、エラーケース
- `pack` / `unpack` / `pack-bucket` / `patch` の一覧とファイルの md5
- キャビネット・`~/.cfs`・出力ディレクトリの全ファイルの md5
- `cfs http` のレスポンス（ファイル・ディレクトリ一覧・リダイレクト）

## 制約

- ダウンロードは `static-server/` 経由の HTTP。Windows では `file://` のダウンロードが元から動かないため（`Url` なしの `file://` ケースは 1 つだけ記録している）
- panic はスタックトレースのパスが変わるので、標準エラーの先頭 1 行だけを比較する
- S3 / GCS は対象外（`internal/cfs/*_storage_test.go` の偽サーバーテストで確認する）
- 時刻はローカルタイムゾーンで出力されるので、比較は同じマシンで行う
