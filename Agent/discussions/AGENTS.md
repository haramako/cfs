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

- 2026-10-25 [暗号鍵の環境変数での上書きと認証方法の説明（計画）](2026-10-25-encrypt-key-env-override-plan.md) — 認証情報は SDK 標準で問題なし。暗号鍵を `.cfsenv` 以外（環境変数）から渡せるようにする計画。認証方法の説明は `docs/storage-backends.md` に記載済み
- 2026-10-25 [S3 から HTTP 公開なしで sync する計画（検討中）](2026-10-25-s3-direct-sync-plan.md) — `Downloader` の取得部分を `Fetcher` に切り出し、`Url` が `s3://` のときだけ GetObject / HeadObject で取得する案。`Url` 未指定時の挙動は据え置き。実装は追加相談の後
- 2026-10-24 [Go モジュール構成と依存の現代化](2026-10-24-modernize-go-layout.md) — `internal/` へ移動（CLI 専用）、go 1.26、GitHub Actions + goreleaser v2、aws-sdk-go-v2 / cloud.google.com/go/storage / urfave/cli v3 / yaml v3。既知の不具合は挙動維持のため据え置き
- 2026-09-05 [castle で育った Agent/ 規約を雛形へ逆移植](2026-09-05-port-castle-agent-rules.md) — `@Agent/AGENTS.md` 強制ロード・discussions 自動記録・`scripts/` 領域（記事はコードが正・TODO 昇格先 wiki はオーナーが fa21532 で先に反映）。`[AI]` 規約は据え置き

