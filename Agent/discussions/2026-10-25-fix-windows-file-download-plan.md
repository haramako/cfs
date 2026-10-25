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
