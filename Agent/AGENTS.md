# Agent Workspace

## Purpose

エージェント（Claude 等）の作業成果を蓄積する領域。コードベースには載らない知識・歴史・Issue 記録・補助スクリプトを、目的別のサブ領域へ整理する。プロダクションコードは置かない。

## Ownership

- `wiki/` … 各ディレクトリの `AGENTS.md` に載らない**横断的なコードベース知識**の記事（`wiki/AGENTS.md`）
- `discussions/` … ユーザーとの**会話の歴史**（相談・決定事項・経緯）（`discussions/AGENTS.md`）
- `issues/` … GitHub Issue / multica などの**Issue ごとの記録**（`issues/AGENTS.md`）
- `scripts/` … エージェントが作成した補助スクリプト。用途別のサブフォルダに分ける（`scripts/AGENTS.md`）
- `TODO.md` … セッションをまたぐ **Claude 中心の継続 TODO**（issue 化するほどではないもの。短く保つ）

## Local Contracts

- ここの内容はプロジェクトのビルド/実行に影響しない
- 知識・作業の置き場所の使い分け:
  - **特定ディレクトリに閉じた知識** → そのディレクトリの `AGENTS.md`
  - **複数ディレクトリにまたがる知識** → `wiki/`
  - **「なぜそうしたか」という決定の経緯** → `discussions/`
  - **特定 Issue の作業記録** → `issues/`
  - **エージェント作成の解析・検証スクリプト** → `scripts/<用途>/`
  - **セッションをまたぐ継続 TODO**（issue 化しないもの） → `TODO.md`
- `TODO.md` は短く保つ。項目が増え続けるのはタスク滞留のサインなので、増えたら消化を優先し、必要なら `issues/` へ昇格させる
- セッション内だけの一時タスクはハーネスの Task ツール（揮発）で扱い、`TODO.md` には残さない
- **会話の中で重要な決定・知見・設計選択が出たら、エージェントは自動で `discussions/` に記録する**（指示を待たない。トリガー文は root の `AGENTS.md` にもある＝毎セッション発火する面はそちら）

## Child DOX Index

- `wiki/AGENTS.md` — 横断的コードベース知識の記事置き場（規約 + 記事インデックス）
- `discussions/AGENTS.md` — ユーザーとの会話・決定事項の歴史ログ
- `issues/AGENTS.md` — Issue トラッキング（multica / GitHub）ごとの記録
- `scripts/AGENTS.md` — エージェント作成スクリプトの配置・実行規約
