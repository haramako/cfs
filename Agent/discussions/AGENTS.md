# Discussions

## Purpose

エージェントとユーザーの会話における**相談内容・決定事項・経緯**を歴史ログとして蓄積する。Wiki や各 `AGENTS.md`（＝「現在の正しい状態」を書く場所）には載らない、**「なぜそうなったか」という歴史的事項**を残す。

## Local Contracts

- 1トピックにつき1ファイル。ファイル名は `YYYY-MM-DD-<slug>.md`（`<slug>` は内容を表す kebab-case。例: `2026-06-21-dox-tree-bootstrap.md`）
- 日付はその相談・決定が行われた日
- 推奨内容: 背景／相談したこと／選択肢と検討／**決定事項**／理由
- **追記型・不変**: 過去ログは原則書き換えない。方針が変わったら新しい日付のファイルを追加する
- 決定がコードや運用の現在ルールに昇格する場合は、該当する `AGENTS.md`（または `../wiki/`）にも反映し、ここには経緯として残す
- ファイルを追加したら、この `AGENTS.md` の **Log** インデックスを更新する（新しい順）

## Log

- 2026-10-25 [リリース用ワークフローの動作確認](2026-10-25-release-workflow-check.md) — `v0.0.0-test.1` で成功（プレリリース、Latest は変わらず）。`prerelease: auto` を追加、goreleaser-action を v7 に更新（v7 は未実行）。確認用リリースとタグは削除済み
- 2026-10-25 [S3 のリージョンを標準的に指定できるようにする（計画）](2026-10-25-s3-region-plan.md) — `.cfsenv` の `S3Region` → バケットから自動判定 → `AWS_REGION` / プロファイル → `ap-northeast-1` の順。静的サイト URL もリージョンから組み立てる
- 2026-10-25 [対応ストレージをローカルと S3 に絞る](2026-10-25-storage-scope-file-and-s3-only.md) — **決定**: 当面は `file://` と `s3://` だけ。GCS などは実装から外してよいが、後から足せる設計は保つ。機能を作り直してから再対応
- 2026-10-25 [Windows で file:// から取得できない不具合の修正（計画）](2026-10-25-fix-windows-file-download-plan.md) — **修正する方針で決定**。原因は `http.Dir("")` による相対パス解決。`file://` は `Fetcher` の file 実装で直接読む。スキップ中の `TestCompress` を有効にして windows CI で確認する。追記: `Cabinet` に普通のパスも書けるようにする（相対は `.cfsenv` 基準）。`C:hoge` の形はエラーにする
- 2026-10-25 [暗号化の改善（計画）](2026-10-25-encryption-improvement-plan.md) — `.ab` 無暗号・IV 固定・改ざん対策なしを確認。A 署名+HTTPS、B 未公開コンテンツの鍵を後から配る、C ファイルごとの IV と鍵の入れ替え、D 配信の認証、E 難読化。A と C は cfs だけで実装、B は別件。**互換性は当面不要（オーナー指示）**。追記: `.ab` 無暗号は実行時コストのため意図的
- 2026-10-25 [暗号鍵の環境変数での上書きと認証方法の説明（計画）](2026-10-25-encrypt-key-env-override-plan.md) — 認証情報は SDK 標準で問題なし。暗号鍵を `.cfsenv` 以外（環境変数）から渡せるようにする計画。認証方法の説明は `docs/storage-backends.md` に記載済み
- 2026-10-25 [S3 から HTTP 公開なしで sync する計画（検討中）](2026-10-25-s3-direct-sync-plan.md) — `Downloader` の取得部分を `Fetcher` に切り出し、`Url` が `s3://` のときだけ GetObject / HeadObject で取得する案。`Url` 未指定時の挙動は据え置き。実装は追加相談の後
- 2026-10-24 [Go モジュール構成と依存の現代化](2026-10-24-modernize-go-layout.md) — `internal/` へ移動（CLI 専用）、go 1.26、GitHub Actions + goreleaser v2、aws-sdk-go-v2 / cloud.google.com/go/storage / urfave/cli v3 / yaml v3。既知の不具合は挙動維持のため据え置き
- 2026-09-05 [castle で育った Agent/ 規約を雛形へ逆移植](2026-09-05-port-castle-agent-rules.md) — `@Agent/AGENTS.md` 強制ロード・discussions 自動記録・`scripts/` 領域（記事はコードが正・TODO 昇格先 wiki はオーナーが fa21532 で先に反映）。`[AI]` 規約は据え置き

