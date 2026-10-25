# TODO

セッションをまたいで残す、Claude 中心の継続作業メモ。issue 化するほどではないが忘れたくないものを置く。

**短く保つ前提**（項目が増えてきたらタスクが滞っているサイン。消化を優先する）。

- 完了した項目は削除する
- issue 化すべきものは `issues/` へ、決定事項は `discussions/` へ、恒久的な事実・ゴールは `wiki/` へ移す
- 形式は `- [ ] <内容>`（必要なら末尾に `(YYYY-MM-DD)` で追加日）

## Items

- [ ] `cfs http` を `cfs serve` に改名し、公開サーバーで動かせるようにする（認証は oauth2-proxy、XSS などの修正、ストリーミング・Range・キャッシュの性能改善）。計画は [discussions/2026-10-25-public-serve-plan.md](discussions/2026-10-25-public-serve-plan.md) (2026-10-25)
- [ ] S3 のリージョンを標準的に指定できるようにする（固定の `ap-northeast-1` をやめる）。計画は [discussions/2026-10-25-s3-region-plan.md](discussions/2026-10-25-s3-region-plan.md) (2026-10-25)
- [ ] GCS（`gs://`）を実装から外す（当面はローカルと S3 だけ、後から足せる設計は保つ）。方針は [discussions/2026-10-25-storage-scope-file-and-s3-only.md](discussions/2026-10-25-storage-scope-file-and-s3-only.md) (2026-10-25)
- [ ] Windows で `file://` のキャビネットから取得できない不具合を直す（修正方針で決定）。あわせて `Cabinet` に普通のパス（`./cabinet` など、相対は `.cfsenv` 基準）を書けるようにする。計画は [discussions/2026-10-25-fix-windows-file-download-plan.md](discussions/2026-10-25-fix-windows-file-download-plan.md) (2026-10-25)
- [ ] 暗号化を改善する（ファイル一覧への署名、ファイルごとの IV など）。計画は [discussions/2026-10-25-encryption-improvement-plan.md](discussions/2026-10-25-encryption-improvement-plan.md)。`.ab` が無暗号なのは意図的（実行時の復号コストのため）と確認済み。A（署名）→ C（IV）の順 (2026-10-25)
- [ ] 暗号鍵を環境変数（例: `CFS_ENCRYPT_KEY`）で上書きできるようにする。計画は [discussions/2026-10-25-encrypt-key-env-override-plan.md](discussions/2026-10-25-encrypt-key-env-override-plan.md) (2026-10-25)
- [ ] S3 から HTTP 公開なしで sync できるようにする。計画は [discussions/2026-10-25-s3-direct-sync-plan.md](discussions/2026-10-25-s3-direct-sync-plan.md)。オーナーの追加相談を受けてから実装する (2026-10-25)
