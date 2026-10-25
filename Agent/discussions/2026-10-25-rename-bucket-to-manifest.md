# cfs の「バケット」を「マニフェスト」に改名する

**状態: 方針として決定（2026-10-25 オーナー指示）。実装は未着手。**

## 背景

cfs では、1回のアップロードで作られる「ファイル一覧」（パスとハッシュの対応表）を**バケット**と呼んでいる。これが S3 や GCS の「バケット」とまぎらわしく、GC の設計の相談でも「どのバケットのことか」が分かりにくくなった。互換性を当面気にしなくてよい（[オーナー指示](2026-10-25-encryption-improvement-plan.md)）ので、今のうちに名前を変える。

## 決定事項

- cfs の「バケット」を **「マニフェスト」（manifest）** に改名する
- S3 の「バケット」は、そのまま「バケット」と呼ぶ

## 改名の対象（実装時のメモ）

- **キャビネット上の置き場所**: ファイル一覧を `data/` から分けて `manifest/xx/yyyy...` に置く（[GC の計画](2026-10-25-garbage-collection-plan.md) の形式変更と同時に行う）
- **コード**: `type Bucket` → `Manifest`、`NewBucket`、`BucketFromFile`、`Client.UploadBucket`、`Downloader.LoadBucket`、`ExistsAll` / `FetchAll` / `Sync` の引数、`cmd/cfs` の `filterBucket` / `packFromBucket` / `doPackBucket`、`"*bucket*"` という内部のファイル名など
  - S3 の `BucketName` や aws-sdk の `Bucket` フィールドは S3 のバケットなので**変えない**
- **CLI**:
  - `upload` の `--bucket` / `-b`（手元のファイル一覧の保存先）→ `--manifest` / `-m` などに変える
  - `pack-bucket` コマンド → `pack-manifest` などに変える
  - ヘルプの文言（`merge buckets`、`list files in bucket` など）
- **手元のキャッシュ**: `~/.cfs/cache/` の中身もマニフェストなので、説明を合わせる
- **ドキュメント**: `docs/`（`overview.md` の用語表、`commands.md`、`configuration.md`、`limitations.md` など）、wiki、ゴールデンテスト（`Agent/scripts/behavior-golden/golden.sh` の `pack-bucket` など）
- テスト名（`TestNewBucket` など）

## 進め方

単独でやると広い範囲の名前を変えるだけの変更になるので、**GC の形式変更（`manifest/` への分離）と同じタイミング**か、その直前に、名前だけを変える1つの変更として行うのがよい。
