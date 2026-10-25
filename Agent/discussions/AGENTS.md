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

- 2026-10-25 [大きなパックファイルと差分の扱い（アイデアの相談）](2026-10-25-large-pack-delta-ideas.md) — 類似アプリ（CCD・ORAS・ostree・casync など）と差分方式の比較。AssetBundle は LZ4 なので変更はファイル単位で別物になり、チャンク差分は効果が薄い（オーナーの結論）。残る課題は小さなファイルをまとめて取ること（パック + Range 案）
- 2026-10-25 [暗号化・圧縮しないファイルを、ファイル名のパターンで設定できるようにする](2026-10-25-no-encrypt-no-compress-patterns.md) — `.cfsenv` に `NoEncrypt` / `NoCompress`（`Exclude` と同じ書き方）。既定は空にし、決め打ちの `.ab` `.raw` `.pbx` `.mp4` はやめた
- 2026-10-25 [sync に、一覧にないファイルを消す --delete を足す](2026-10-25-sync-delete-option.md) — **決定**: 既定では消さず、`--delete` で消す（出力先を間違えたときの被害が大きいため。rsync や aws s3 sync と同じ）。書き出しが全部うまくいった後に消す。Windows と macOS では大文字・小文字を区別しない
- 2026-10-25 [パックファイルのパッチで、ファイルの削除を表せるようにする](2026-10-25-pack-patch-deletions.md) — **決定**: 形式を 3 に上げ、ファイル一覧の後に「消すファイルの一覧」を足す（削除の印のエントリ案は、古い読み手が気づけないのでやめた）。`unpack -o` がパッチを当て、消すファイルを消す。`..` や絶対パスはエラー
- 2026-10-25 [アップロードの対象から外すファイルを設定できるようにする](2026-10-25-upload-exclude-patterns.md) — **決定**: いつも外すのは `.*` `#*` `*~` だけ、それ以外は `.cfsenv` の `Exclude` に `.gitignore` と似たパターンで書く。`!` で戻せる、フォルダは中身ごと外す、ファイルの直接指定には当てない
- 2026-10-25 [0 バイトのデータもキャビネットに置く](2026-10-25-upload-zero-byte-data.md) — 特別扱いは昔の S3 ライブラリが 0 バイトで失敗したため。aws-sdk-go-v2 では問題ないことを実 S3 で確認。**決定**: 置く、GC と `ls --verify` でも普通に扱う、取得の近道（空の SHA-256 なら取りに行かない）は残す
- 2026-10-25 [リファクタリング（設定を引数で渡す・CLI のエラー処理・ダウンロードの整理）](2026-10-25-refactoring-config-and-errors.md) — 設定は `cfs.Config` を引数で渡す（グローバル変数をやめた）。`~/.cfs_setting` をやめ、キャッシュは `os.UserCacheDir()/cfs`（`CFS_CACHE_DIR`）。CLI はエラーを返し、終了コード 1。`Sync` の並列化、`--filter-cmd` の引用符、キャッシュ名は SHA-256
- 2026-10-25 [キャビネットを作るコマンド（cfs cabinet init）の計画](2026-10-25-cabinet-init-plan.md) — S3 のバケット作成（非公開・ACL 無効・バージョニングなし）とローカルのフォルダ作成、`config.json` に形式の版を入れる。追記: 未決事項はすべて了承され、`cabinet init` と `cabinet policy` を実装した
- 2026-10-25 [計画 1〜8 の実装結果](2026-10-25-implementation-results.md) — GCS 削除、Fetcher と Windows 修正、S3 直接取得とリージョン、暗号鍵の環境変数、マニフェストへの改名、形式 v2（SHA-256・AES-256-CTR・署名・GC）、`cfs serve`。計画からの変更点と残り（Unity 側、実 S3 での確認、署名鍵の置き場所、serve のストリーミング）
- 2026-10-25 [実装前の確認事項への回答](2026-10-25-implementation-decisions.md) — **決定**: 実装の順番、形式変更は「形式 v2」にまとめる、S3 直接取得を既定に、環境変数名・CLI 名・タグ名の規則・`S3Region`、GC の保持期間はキャビネットに置く、`.htm` の特別扱いはやめる。各計画の本文よりこちらが優先
- 2026-10-25 [cfs の「バケット」を「マニフェスト」に改名する](2026-10-25-rename-bucket-to-manifest.md) — **決定**: S3 のバケットとまぎらわしいため。置き場所は `manifest/`。GC の形式変更と同時かその直前に行う
- 2026-10-25 [ストレージのガーベージコレクション（計画）](2026-10-25-garbage-collection-plan.md) — ファイル一覧を `bucket/` に分け、全タグ + 保持期間内の一覧を起点にマーク。未参照・保持期間より古い・2回連続候補のものだけ消す。読めない一覧があれば中止。タグ整理は `cfs tag ls/rm`。追記で変更: 置き場所は `manifest/`、GC は「マニフェストが先・中身は次の回」、`upload` は前回のマニフェストの存在を1回確かめてキャッシュを使う
- 2026-10-25 [cfs http を公開サーバーで動かす計画（認証・安全性・性能）](2026-10-25-public-serve-plan.md) — 追記で決定: 認証は oauth2-proxy、メールアドレス一覧は非公開のデプロイ用リポジトリ、`cfs serve` に改名。用途は HTML などのアーティファクトの閲覧で、性能改善は一覧キャッシュと ETag を最優先に見直し。認証は前段で（案1 Cloud Run + IAP、案2 oauth2-proxy）。取引先は自前 OAuth クライアント（外部）で通す。XSS・待ち受け・panic・タイムアウトの修正と、ストリーミング・Range・一覧キャッシュの性能改善
- 2026-10-25 [リリース用ワークフローの動作確認](2026-10-25-release-workflow-check.md) — `v0.0.0-test.1` で成功（プレリリース、Latest は変わらず）。`prerelease: auto` を追加、goreleaser-action を v7 に更新（v7 は未実行）。確認用リリースとタグは削除済み
- 2026-10-25 [S3 のリージョンを標準的に指定できるようにする（計画）](2026-10-25-s3-region-plan.md) — `.cfsenv` の `S3Region` → バケットから自動判定 → `AWS_REGION` / プロファイル → `ap-northeast-1` の順。静的サイト URL もリージョンから組み立てる
- 2026-10-25 [対応ストレージをローカルと S3 に絞る](2026-10-25-storage-scope-file-and-s3-only.md) — **決定**: 当面は `file://` と `s3://` だけ。GCS などは実装から外してよいが、後から足せる設計は保つ。機能を作り直してから再対応
- 2026-10-25 [Windows で file:// から取得できない不具合の修正（計画）](2026-10-25-fix-windows-file-download-plan.md) — **修正する方針で決定**。原因は `http.Dir("")` による相対パス解決。`file://` は `Fetcher` の file 実装で直接読む。スキップ中の `TestCompress` を有効にして windows CI で確認する。追記: `Cabinet` に普通のパスも書けるようにする（相対は `.cfsenv` 基準）。`C:hoge` の形はエラーにする
- 2026-10-25 [暗号化の改善（計画）](2026-10-25-encryption-improvement-plan.md) — `.ab` 無暗号・IV 固定・改ざん対策なしを確認。A 署名+HTTPS、B 未公開コンテンツの鍵を後から配る、C ファイルごとの IV と鍵の入れ替え、D 配信の認証、E 難読化。A と C は cfs だけで実装、B は別件。**互換性は当面不要（オーナー指示）**。追記: `.ab` 無暗号は実行時コストのため意図的
- 2026-10-25 [暗号鍵の環境変数での上書きと認証方法の説明（計画）](2026-10-25-encrypt-key-env-override-plan.md) — 認証情報は SDK 標準で問題なし。暗号鍵を `.cfsenv` 以外（環境変数）から渡せるようにする計画。認証方法の説明は `docs/storage-backends.md` に記載済み
- 2026-10-25 [S3 から HTTP 公開なしで sync する計画（検討中）](2026-10-25-s3-direct-sync-plan.md) — `Downloader` の取得部分を `Fetcher` に切り出し、`Url` が `s3://` のときだけ GetObject / HeadObject で取得する案。`Url` 未指定時の挙動は据え置き。実装は追加相談の後
- 2026-10-24 [Go モジュール構成と依存の現代化](2026-10-24-modernize-go-layout.md) — `internal/` へ移動（CLI 専用）、go 1.26、GitHub Actions + goreleaser v2、aws-sdk-go-v2 / cloud.google.com/go/storage / urfave/cli v3 / yaml v3。既知の不具合は挙動維持のため据え置き
- 2026-09-05 [castle で育った Agent/ 規約を雛形へ逆移植](2026-09-05-port-castle-agent-rules.md) — `@Agent/AGENTS.md` 強制ロード・discussions 自動記録・`scripts/` 領域（記事はコードが正・TODO 昇格先 wiki はオーナーが fa21532 で先に反映）。`[AI]` 規約は据え置き

