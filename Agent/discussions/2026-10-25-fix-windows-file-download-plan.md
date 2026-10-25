# Windows で file:// のキャビネットから取得できない不具合の修正（計画）

**状態: 修正する方針で決定（2026-10-25 オーナー指示）。実装は未着手。**

## 現象

Windows で、`Url` を指定しない `file://` のキャビネット（例: `file:///C:/work/cabinet`）に対して `sync` / `ls` / `cat` などを実行すると、`tag/` や `data/` の取得が 404 になって失敗する。アップロードは動く。2026-10-24 のゴールデンテストで、構成変更前のバイナリでも同じだったことを確認済み（既存の不具合）。

## 原因

`internal/cfs/downloader.go` の `getRequest` は、`file://` を `http.NewFileTransport` で扱っている。Windows では `http.Dir("")` を使っているため、URL のパス `/C:/work/cabinet/tag/v1` が `.\C:\work\cabinet\tag\v1` のような**今のフォルダからの相対パス**として解決され、ファイルが見つからない（Linux / macOS は `http.Dir("/")` なので動く）。

## テストで見つからなかった理由

`file://` からダウンロードするテストは `internal/cfs/bucket_test.go` の `TestCompress` だけで、これは「`diff` コマンドがないとだめなため」`t.Skip()` されている。そのため CI の windows でも検出されなかった。

## 修正方針

- `file://` は HTTP の仕組みを通さず、**URL をローカルのパスに変換して `os.ReadFile` / `os.Stat` で直接読む**
  - Windows のドライブ文字（`file:///C:/...` → `C:\...`）と、共有フォルダ（`file://server/share/...` → `\\server\share\...`）を扱う
  - ファイルがないときは、HTTP の 404 と同じ扱いのエラーにする（`ls --verify` の `ng` 判定、`FetchAll` のリトライも同じ動きにする）
  - URL からパスへの変換は、`FileStorage` にある同様の処理（先頭の `/` を落とす部分）と共通の関数にする
- [S3 から直接 sync する計画](2026-10-25-s3-direct-sync-plan.md) で `Downloader` の取得部分を `Fetcher` に切り出す予定なので、**その `file://` 版として実装する**のが素直。S3 の計画より先にやる場合も、同じ形（`Fetcher` の file 実装）で入れておく
- `TestCompress` を、外部の `diff` コマンドを使わず Go で中身を比べる形に直して有効にする。CI の windows と ubuntu の両方で、アップロード → `file://` からのダウンロードが通ることを確認する
- 修正後、`docs/limitations.md` と wiki の `known-issues-and-gotchas.md` からこの制限を消し、ゴールデンテストの「`Url` なしの `file://`」のケースが成功するようになったことを確認する（意図した挙動の変化）

## 互換性

当面は互換性を気にしなくてよい（2026-10-25 オーナー指示）。今は失敗するだけの動作なので、直しても困る利用者はいない。

## 追記（2026-10-25）: 普通のパスも書けるようにする

オーナーの指示で、この修正に合わせて次の変更も入れる。URL とパスの変換という同じ場所の話なので、一緒に直すのが効率的。

### 背景

`file://` の標準（RFC 8089）は `file://<ホスト>/<絶対パス>` で、**相対パスは書けない**。`file://./x` はホスト名が `.` になるだけで、`file:path`（SQLite が相対パスとして受け付ける形）も RFC の文法からは外れる。独自記法を作るより、URL ではなく普通のパスをそのまま受け付けるほうが一般的（git のリモート、rclone、Terraform など）。

### 方針

- `Cabinet`（と `Url`、`--cabinet`）は、**`://` が入っていれば URL、入っていなければ手元のパス**として扱う

  ```json
  { "Cabinet": "./cabinet" }
  { "Cabinet": "../shared/cabinet" }
  { "Cabinet": "C:\\work\\cabinet" }
  { "Cabinet": "s3://my-bucket" }
  ```

- `file:///C:/...` などの `file://` URL も、今までどおり受け付ける
- **相対パスの基準**
  - `.cfsenv` に書いた相対パスは、**`.cfsenv` があるフォルダ**を基準にする（設定ファイル内の相対パスは設定ファイル基準、という多くのツールの慣習。`cfs -C sub/.cfsenv upload` のように別の場所から実行しても、アップロード先が変わらない）
  - コマンドラインの `--cabinet ./x` は、今いるフォルダを基準にする
- **Windows のドライブ文字に注意**: Go の `url.Parse` に `C:\work` を渡すと、`C` をスキームと解釈してしまう。URL として解析する前に「英字1文字 + `:` で始まるならパス」と判定する
- 内部では、パスを絶対パスにしてから、本文の修正方針で作る「URL ↔ ローカルのパス」の変換と同じ扱いにする

### テスト

- 相対パス（`.cfsenv` 基準とコマンドライン基準）、絶対パス、Windows のドライブ文字、`file://` URL、`s3://` の判定をテストする
- CI の windows と ubuntu の両方で、相対パスのキャビネットにアップロード → 取得が通ることを確認する

### ドキュメント

- `docs/configuration.md` と `docs/storage-backends.md` の `Cabinet` の説明に、普通のパスで書けることと相対パスの基準を書く
- ルートの `README.md` の「はじめかた」は、ローカルで試す例を `"Cabinet": "./cabinet"` のような書き方にできる
