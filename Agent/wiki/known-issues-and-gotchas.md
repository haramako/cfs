# 既知の不具合と落とし穴

コードを読んで見つけたもの（2026-10-24 時点、未修正）。修正したらこの記事から消すこと。表のファイル名は特記がなければ `internal/cfs/` 基準。

## 挙動上の注意

- **0 バイトのファイル**: S3 は 0 バイトを Put するとエラーになるので送らない。ダウンロード側は `Size == 0` のとき取得せずに空ファイルを作る（`Downloader.Sync` / `FetchAll`）。0 バイトに関する修正は何度も入っているので、ここを触るときは S3 での挙動も確認する
- **除外ルールが 2 系統ある**: `Client.AddFiles` にハードコードされたもの（名前が `.` / `#` で始まる、`~` で終わる、拡張子 `.meta` `.manifest` `.tmx`）と、`ExcludePatterns`（`.*` `*.vdat` `cfs` `*.meta` `*.tmx`）。後者は `filepath.Match` をパス全体に当てているため、`*` がディレクトリ区切りにマッチせず、フォルダ引数なし（ルートが `.`）で実行したときの直下にしか効かない（`cfs upload src` のときは `src/x.vdat` も対象になる。実測済み）
- **`.` で始まるディレクトリの中身はアップロードされる**: ハードコード側の除外はディレクトリに対して `SkipDir` ではなく `nil` を返すので、`.hidden/x.txt` のようなファイルは対象になる（ゴールデンテストで確認済み）
- **Windows では `file://` からのダウンロードが動かない**: `getRequest` が Windows で `http.Dir("")` を使うため、`file:///C:/...` の `tag/` や `data/` が 404 になる。`Url` を指定しない `file://` キャビネットでは `ls` / `sync` などが失敗する（アップロードは動く）
- **バケットはスナップショット**: `upload` は今回見つからなかったファイルをバケットから消す。複数ディレクトリを 1 回で上げると、各ルートからの相対パスがキーになるので同名パスは衝突する
- **`sync` は出力先の余分なファイルを消さない**。また全ファイルを毎回書き直す（データ取得はキャッシュされるが、書き込みは逐次）
- **ダウンロード側の設定一致**: バケット自体は `DefaultContentAttribute()` でデコードされるので、ダウンロード側の `.cfsenv` の `Compress` / `EncryptKey` / `EncryptIv` がアップロード側と一致していないとバケットが読めない
- **無圧縮拡張子はハードコード**: `.ab` `.raw` `.pbx` `.mp4`（`Bucket.GetAttribute`）
- **S3**: リージョン `ap-northeast-1` 固定、ダウンロード URL は S3 静的サイトの http エンドポイント
- アップロードワーカーはエラー時に panic する（不完全なバケットをアップロードしないための意図的な挙動。2021-03 の修正）

## 不具合と思われるもの

| 場所 | 内容 |
|---|---|
| `config.go` `GlobalCacheDir` / `GlobalDataCacheDir` / `GlobalSettingPath` | 関数内で `globalCacheDir := ...` と書いておりパッケージ変数をシャドーしているため、結果がキャッシュされない。呼ぶたびに `InitDefaultSetting` が `~/.cfs_setting` を読み直す（`Fetch` はファイルごとに呼ぶので無駄が大きい） |
| `file_storage.go` `Upload` | `os.Stat` 後に `os.IsExist(err)` を見ているが、存在時は `err == nil` なので常に上書きしている（内容は同じなので実害は小さい） |
| `gcs_storage.go` `Upload` | `if err != nil && googleapi.IsNotModified(err)` のため、304 以外のエラー（認証失敗など）が握りつぶされる。2026-10 のライブラリ移行でも挙動維持のため意図的にそのまま残している |
| `util.go` `encode` | 引数 `encrypt_key` / `encrypt_iv` を無視して `Option` を直接使っている（`decode` は引数を使う） |
| `bucket.go` `Parse` | 空行判定が `len(s)`（全体）になっている。列数チェックが `>= 3` なのに `col[6]` まで参照するので、3〜6 列の壊れた行で panic する |
| `cmd/cfs/main.go` | `--setting` は未使用、`--url` は未定義なのに `GlobalString("url")` を読んでいる |
| `cmd/cfs/cmd_patch.go` | `pack.Patch` のエラーを無視している |
| `internal/pack/pack.go` `Parse` | データ読み込みに `io.ReadFull` ではなく `r.Read` を使っており、ストリームによっては短く読まれてエラーになりうる。パス長は 1 バイトなので 255 バイト超のパスは壊れる |
| `README.md` | コマンド一覧・設定の記述が古く TODO が残っている。正しい情報は [cli-and-configuration.md](cli-and-configuration.md) |
