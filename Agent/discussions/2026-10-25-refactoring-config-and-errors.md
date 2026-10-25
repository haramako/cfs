# リファクタリング（設定を引数で渡す・CLI のエラー処理・ダウンロードの整理）

**状態: 実装済み（2026-10-25）。**

## 背景

テストを増やした（カバレッジ 43.5% → 87.1%）後、オーナーから「リファクタしたほうがいいところはある？」と聞かれ、エージェントが優先順に 5 つ挙げた。オーナーの回答:

1. CLI のエラー処理 → 直す
2. 使われていない設定 → 不要そうなものは消してよい
3. ダウンロードまわり → 整理する
4. グローバル状態 → 引数で渡す形にする
5. ストリーミング → 後回し
- `Manifest` のレシーバー名 → `m` に揃える
- キャッシュのファイル名（`escapeFilename` + MD5）→ やり方は任せるので直す

## 決定事項と、エージェントが決めたこと

- **設定は `cfs.Config` にまとめ、引数で渡す**。グローバル変数の `Option` / `Verbose` / `Setting` をなくした。`Client.Config`、`NewDownloader(location, cfg)`、`OpenStorage` / `OpenFetcher(location, cfg)`、`GCOptions.Keys` / `Log`。ログは `Logger`（nil なら出さない）
- **`~/.cfs_setting`（`HomeRoot`）と `cfs setting`、`--setting` を消した**。キャッシュは `os.UserCacheDir()/cfs`（Windows は `%LocalAppData%\cfs`）に移し、場所は環境変数 `CFS_CACHE_DIR` で変える（`HomeRoot` の用途だった「別のドライブに移す」はこれで足りる）。中は `manifest/` と `data/`。YAML の依存もなくなった
- **使われていない `.cfsenv` のキー `Tag` / `Repository` と、`NoCache` を消した**（`NoCache` は `--force` と同じ。書かれていても無視される）
- **`Cabinet` の既定値を `file:///var/cfs` から空にした**。空のままキャビネットを使うと `no cabinet` エラー
- **CLI のエラー**: 各コマンドがエラーを返し、`main` が標準エラーに `error: ...` を出して終了コード 1。`check()` の panic と `os.Exit` はやめた。`Client` のアップロードの失敗も panic ではなくエラー（マニフェストは置かない）。`ls --verify` は `ng` があれば終了コード 1
- **`.cfsenv` が JSON として壊れていたらエラー**（以前は表示して続けていた）
- **マニフェストの読み込みは属性をデータから判断する**（GC と同じ `decodeManifest`）。ダウンロード側の `Compress` が違っても読める
- **ダウンロード**: `Sync` を 8 並列にし、`FetchAll` / `ExistsAll` と同じく errgroup で書いた。取得のやり直し（3 回）は `Sync` にも効く。データのキャッシュの書き込みの失敗は無視する（同じ中身の同時書き込みで Windows の置き換えが失敗するため）
- **`--filter-cmd`**: マニフェストとパックで共通の `filterPaths` にした。シェルを通さず、`"..."` / `'...'` で空白を含む引数を渡せる。フィルターの失敗は cfs の失敗
- **キャッシュのファイル名**: キャビネット・今のフォルダ・上げるフォルダの絶対パスを NUL 区切りで SHA-256（`uploadCachePath`）。`escapeFilename` と MD5 は消した
- **`internal/pack` は `cfs` に依存しない**ようにした（`Patch` のログをやめ、`patch -V` は cmd 側で変わったファイルを出す）

## 互換性

使っているプロジェクトはまだないので、互換性は気にしない（オーナー指示、memory の no-backward-compat-yet）。キャッシュの場所が変わったので、古い `~/.cfs` は手で消してよい。
