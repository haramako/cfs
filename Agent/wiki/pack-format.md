# パックファイル（TP 形式）とパッチ

2019-08 に追加された、複数ファイルを 1 ファイルにまとめる独自形式（`internal/pack/pack.go`）。キャビネット（内容アドレス型ストレージ）とは独立していて、アプリ同梱やまとめて配布する用途向け。拡張子は CLI の usage では `.cfspack` / `.tp` の両方が出てくるが、形式は同じ。

## バイナリレイアウト

すべてリトルエンディアン。

```
header        3 bytes   'T' 'P' version(=1)
listSize      uint32    エントリリスト部のバイト数
entryList:
  count       uint32
  entry × count:
    pathLen   uint8     （パスは最大 255 バイト）
    path      pathLen bytes, UTF-8, '/' 区切り
    pos       uint32    ファイル先頭からのデータ位置（絶対オフセット）
    size      uint32
    hash      16 bytes  元データの MD5
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
| `cfs pack-bucket location out.tp` | バケットの全データを `FetchAll`（8 並列・3 回リトライ）でキャッシュに落としてから、デコードしてパック。ハッシュは `OrigHash` |
| `cfs unpack in.tp` | エントリ一覧（パス, サイズ, ハッシュ）を表示 |
| `cfs unpack in.tp -o dir` | 展開 |
| `cfs patch base.tp current.tp out.tp` | `current` のうち、`base` にないパス、またはハッシュが異なるエントリだけを含むパックを作る |

## パッチの制限

`pack.Patch` は追加・変更のみを表現する。**`base` にあって `current` にないファイル（削除）は表現されない**ので、適用側で削除を扱いたい場合は別の手段が必要。
