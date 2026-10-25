# TODO

セッションをまたいで残す、Claude 中心の継続作業メモ。issue 化するほどではないが忘れたくないものを置く。

**短く保つ前提**（項目が増えてきたらタスクが滞っているサイン。消化を優先する）。

- 完了した項目は削除する
- issue 化すべきものは `issues/` へ、決定事項は `discussions/` へ、恒久的な事実・ゴールは `wiki/` へ移す
- 形式は `- [ ] <内容>`（必要なら末尾に `(YYYY-MM-DD)` で追加日）

## Items

実装の順番と、確認事項への回答は [discussions/2026-10-25-implementation-decisions.md](discussions/2026-10-25-implementation-decisions.md)（各計画の本文よりこちらが優先）。上から順に進める。

- [ ] 1. GCS（`gs://`）を実装から外す（当面はローカルと S3 だけ、後から足せる設計は保つ）。方針は [discussions/2026-10-25-storage-scope-file-and-s3-only.md](discussions/2026-10-25-storage-scope-file-and-s3-only.md) (2026-10-25)
- [ ] 2. 取得部分（`Fetcher`）を切り出し、Windows で `file://` から取得できない不具合を直す。あわせて `Cabinet` に普通のパス（相対は `.cfsenv` 基準、`C:hoge` はエラー）を書けるようにする。計画は [discussions/2026-10-25-fix-windows-file-download-plan.md](discussions/2026-10-25-fix-windows-file-download-plan.md) (2026-10-25)
- [ ] 3. S3 のリージョンを標準的に指定できるようにする（`.cfsenv` の `S3Region` → 自動判定 → `AWS_REGION` → `ap-northeast-1`）。計画は [discussions/2026-10-25-s3-region-plan.md](discussions/2026-10-25-s3-region-plan.md) (2026-10-25)
- [ ] 4. S3 から直接取得する（`Cabinet` が `s3://` で `Url` が空なら既定で直接取得）。計画は [discussions/2026-10-25-s3-direct-sync-plan.md](discussions/2026-10-25-s3-direct-sync-plan.md) (2026-10-25)
- [ ] 5. 暗号鍵を環境変数 `CFS_ENCRYPT_KEY` / `CFS_ENCRYPT_IV` で上書きできるようにする。計画は [discussions/2026-10-25-encrypt-key-env-override-plan.md](discussions/2026-10-25-encrypt-key-env-override-plan.md) (2026-10-25)
- [ ] 6. cfs の「バケット」を「マニフェスト」に改名する（`upload --manifest` / `-m`、`pack-manifest`）。方針は [discussions/2026-10-25-rename-bucket-to-manifest.md](discussions/2026-10-25-rename-bucket-to-manifest.md) (2026-10-25)
- [ ] 7. 形式 v2: `manifest/` への分離、GC（`cfs gc`、保持期間はキャビネットの設定）とタグ整理（`cfs tag ls/rm`）、SHA-256 化、マニフェストへの署名、ファイルごとの IV と鍵 ID。計画は [GC](discussions/2026-10-25-garbage-collection-plan.md) と [暗号化の改善](discussions/2026-10-25-encryption-improvement-plan.md)。署名の秘密鍵の置き場所は後で決める。Unity 側はオーナーからライブラリを受け取ってから合わせる (2026-10-25)
- [ ] 8. `cfs http` を `cfs serve` に改名し、公開サーバーで動かせるようにする（認証は oauth2-proxy、XSS などの修正、一覧キャッシュと ETag を最優先、タグ名は `A-Za-z0-9._-`、`.htm` の Shift_JIS の特別扱いはやめる）。計画は [discussions/2026-10-25-public-serve-plan.md](discussions/2026-10-25-public-serve-plan.md) (2026-10-25)
