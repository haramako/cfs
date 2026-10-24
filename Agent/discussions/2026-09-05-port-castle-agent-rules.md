# castle で育った Agent/ 規約を雛形へ逆移植

## 背景

`Agent/` 雛形を castle（`C:\Work\castle`）に導入した後、castle 側で運用しながら足されたルールが雛形に戻っていなかった。linen-work（`C:\Work\linen-work`）へ新規導入するときに差分が見つかったので、雛形にも反映する。

## 決定事項（雛形に足したもの）

- root `AGENTS.md` 末尾に `@Agent/AGENTS.md` を置き、`Agent/AGENTS.md` を毎セッション強制ロードする（DOX の Read Before Editing は `Agent/` を触らないセッションでは発火しないため）
- 重要な決定・知見が出たらエージェントが自動で `Agent/discussions/` に記録する（root と `Agent/AGENTS.md` の両方に明記）
- `Agent/scripts/<用途>/` 領域を追加（スクリプト・テスト・README を用途ごとにまとめ、製品コードと分離）
- `wiki/` の記事がコードと矛盾したら記事側を直すか削除する（コードが正）／`TODO.md` の昇格先を「メモリ」から `wiki/` に統一 — この2点はオーナーが同日 remote 側で先に反映済み（fa21532「テンプレートを調整」・TODO Items も空にした）。pull で取り込み、こちらの重複編集は捨てた

## 移植しなかったもの

- castle のコミット規約「日本語・`Agent/` 以下だけの変更に `[AI]`」。雛形の例は「全コミットに `[AI]`」で、agent-work 自身の notta-sync コミットもその運用のため、プロジェクトごとの選択として雛形の例は据え置き

## 注意

- 雛形の `Agent/TODO.md` の Items は fa21532 で空になった。それ以前のコピー（linen-work）には agent-work 自身の notta-sync 項目が混入している
