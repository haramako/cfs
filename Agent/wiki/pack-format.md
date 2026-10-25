# パックファイル（TP 形式）とパッチ

2019-08 に追加された、複数ファイルを 1 ファイルにまとめる独自形式（`internal/pack/pack.go`）。キャビネット（内容アドレス型ストレージ）とは独立していて、アプリ同梱やまとめて配布する用途向け。拡張子は CLI の usage では `.cfspack` / `.tp` の両方が出てくるが、形式は同じ。

## バイナリレイアウト

すべてリトルエンディアン。

```
header        3 bytes   'T' 'P' version(=3。2026-10-25 に SHA-256 化で 1→2、同日に削除の一覧を足して 2→3)
listSize      uint32    エントリリスト部 + 削除リスト部のバイト数
entryList:
  count       uint32
  entry × count:
    pathLen   uint8     （パスは最大 255 バイト）
    path      pathLen bytes, UTF-8, '/' 区切り
    pos       uint32    ファイル先頭からのデータ位置（絶対オフセット）
    size      uint32
    hash      32 bytes  元データの SHA-256
deletedList:            パッチで消すパス（PackFile.Deleted）。ふつうのパックでは count=0
  count       uint32
  path × count: pathLen uint8 + path
data          各エントリのデータをパス順に連結（非圧縮・非暗号）
```

- 書き込み時にエントリはパスでソートされる（`Pack`）。`pos` はエントリリストを一度ダミーエンコードしてサイズを測ってから計算している
- データは **デコード済みの生データ**。キャビネットの暗号化・圧縮は関係しない
- ファイルサイズは uint32 なので 4GB 未満が前提
- `Parse` は全データをメモリに読む。`unpack -o` はエントリリストだけ使い、`pos` へ Seek して読み出す

## コマンド

| コマンド | 内容 |
|---|---|
| `cfs pack out.tp dir` | `NewPackFileFromDir` でディレクトリを走査（パスは NFC 正規化）してパック。キャビネットの除外ルールは適用されない |
| `cfs pack-manifest location out.tp` | マニフェストの全データを `FetchAll`（8 並列・3 回リトライ）でキャッシュに落としてから、デコードしてパック。ハッシュは `OrigHash` |
| `cfs unpack in.tp` | エントリ一覧（パス, サイズ, ハッシュ）と、消すパス（`パス\tdeleted`）を表示 |
| `cfs unpack in.tp -o dir` | 展開し、`Deleted` のパスを `dir` から消す（ないものは無視）。`..` や絶対パスは `outputPath`（`filepath.IsLocal`）でエラー |
| `cfs patch base.tp current.tp out.tp` | `current` のうち、`base` にないパス、またはハッシュが異なるエントリと、`base` にあって `current` にないパス（`Deleted`）を含むパックを作る |

## パッチ

`pack.Patch` は追加・変更をエントリに、削除を `Deleted` に入れる（2026-10-25 に削除を足して形式を 3 にした。経緯は `../discussions/2026-10-25-pack-patch-deletions.md`）。パッチを当てるのは、`base` を展開したフォルダへの `unpack -o`。`--filter-cmd` はエントリと `Deleted` の両方を絞る。パッチを当てたパック同士の `patch`（パッチのパッチ）は考えていない（`base.Deleted` は見ない）。
