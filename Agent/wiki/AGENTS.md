# Wiki

## Purpose

各ディレクトリの `AGENTS.md` には載らない、**複数ディレクトリにまたがる横断的なコードベース知識**を記事として蓄積する。

## Local Contracts

- **ここに書くもの**: 設計の全体像、レイヤーをまたぐデータフロー、命名・パターンの慣習、デバッグの勘所など、特定ディレクトリに帰属しない知識
- **ここに書かないもの**: 特定ディレクトリに閉じた知識（→ そのディレクトリの `AGENTS.md`）、会話の経緯・決定理由（→ `../discussions/`）、Issue の作業記録（→ `../issues/`）
- 記事ファイル名は内容を表す kebab-case（例: `data-pipeline-overview.md`）
- 記事を追加・改名・削除したら、この `AGENTS.md` の **Articles** インデックスを更新する
- 記事の内容が特定ディレクトリに帰属すると判明したら、その `AGENTS.md` へ移す
- 記事がコードと矛盾していると気づいたら、記事側を直すか削除する（コードが常に正・古い記事を放置しない）
- 人間向けのドキュメントはリポジトリ直下の `docs/` にある（使い方・設定・ストレージ・制限・開発）。wiki はエージェント向けの内部知識（実装の勘所・不具合の詳細・経緯）に絞り、コマンドや設定の挙動を変えたら `docs/` も合わせて更新する

## Articles

- [architecture-overview.md](architecture-overview.md) — CFS の目的、パッケージ構成、書き込み(Storage)/読み込み(HTTP GET)の非対称性、upload/sync のデータフロー
- [storage-layout-and-bucket-format.md](storage-layout-and-bucket-format.md) — キャビネットの `data/`・`tag/` 配置、バックエンド別 URL、Hash/OrigHash、バケット TSV 形式、圧縮・暗号化と属性
- [cli-and-configuration.md](cli-and-configuration.md) — 全 CLI コマンドとオプション、`--filter-cmd`、`http` サーバー、`.cfsenv` / `~/.cfs_setting`、ローカルキャッシュ
- [pack-format.md](pack-format.md) — TP パックファイルのバイナリレイアウトと pack / unpack / pack-bucket / patch
- [development-build-and-ci.md](development-build-and-ci.md) — モジュール構成、Makefile ビルド、テスト（偽サーバー・ゴールデンテスト）、GitHub Actions + goreleaser v2、開発の歴史
- [known-issues-and-gotchas.md](known-issues-and-gotchas.md) — 挙動上の注意点と、コードから見つけた未修正の不具合一覧

