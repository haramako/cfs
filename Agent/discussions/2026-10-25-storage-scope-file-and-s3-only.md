# 対応ストレージをローカルと S3 に絞る

**状態: 方針として決定（2026-10-25 オーナー指示）。コードからの取り外しは未着手。**

## 決定事項

- 当面、対応するストレージは **ローカルのファイルシステム（`file://`）と Amazon S3（`s3://`）だけ**にする
- それ以外（現状では Google Cloud Storage の `gs://`）は、**実装から外して（無効化して）よい**
- ただし設計は、**後からストレージを足せる前提**を保つ（`Storage` インターフェースと、URL スキームによる選択は残す）
- 他のストレージは、機能（S3 からの直接 sync、Windows の `file://` 修正、暗号化の改善など）を実装し直してから、あらためて対応する

## 理由

- 機能の作り直しを予定しており、対応ストレージが多いと、そのたびに全ストレージで実装・テストし直す必要がある
- まずは2つで機能を固め、形が決まってから他のストレージを足すほうが手戻りが少ない

## 外すときの作業（実装時のメモ）

- `internal/cfs/gcs_storage.go` と `gcs_storage_test.go` を削除する
- `StorageFromUrl` は `gs://` に対して「未対応（今後対応予定）」と分かるエラーを返す。スキームによる選択の `switch` は残す
- `go mod tidy` で `cloud.google.com/go/storage` とその大量の間接依存を外す（ビルドとダウンロードが軽くなる）
- テストの `CFS_TEST_STORAGE=gcs`、Makefile の `test-gcs` を外す
- `docs/`（`storage-backends.md`、`README.md`、`limitations.md` など）、ルートの `README.md`、wiki から GCS の記述を外すか「今後対応予定」にする
- GCS の実装は git の履歴（2026-10-24 の `20d4f1d` で cloud.google.com/go/storage に移行したもの）に残るので、再対応のときの参考にできる

## 他の計画への影響

- [S3 から直接 sync する計画](2026-10-25-s3-direct-sync-plan.md) の `Fetcher` は、file と S3（と HTTP）の実装だけでよい。`gs://` 版は再対応のときに足す
