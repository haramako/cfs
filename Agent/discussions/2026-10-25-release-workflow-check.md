# リリース用ワークフローの動作確認

**状態: 確認済み（2026-10-25）。**

## 背景

2026-10-24 に CircleCI から GitHub Actions + goreleaser v2 へ移行したが、`release.yml` は `v*` タグでしか動かないため、一度も動かしていなかった。オーナーの指示で、意味のないバージョンで一度だけ動かして確認した。

## やったこと

1. `.goreleaser.yml` に `release.prerelease: auto` を追加（`2f19cc6`）。`-` 付きのタグはプレリリースになり、GitHub の「Latest」を奪わない。確認用のタグで最新版の表示が変わらないようにするためで、今後のリリース手順の試験にも使える
2. 注釈付きタグ `v0.0.0-test.1` を master（`2f19cc6`）に打って push
3. Release ワークフロー（run 40162531124）を確認

## 結果

- **成功**（約3分。goreleaser 部分は 1分33秒）
- リリース [v0.0.0-test.1](https://github.com/haramako/cfs/releases/tag/v0.0.0-test.1) が **プレリリース** として作成された。Latest は v0.1.12（2019-11-05）のまま
- 成果物:
  - `cfs_darwin_amd64.zip`（約 13.7MB）
  - `cfs_linux_amd64.zip`（約 13.4MB）
  - `cfs_windows_amd64.zip`（約 13.7MB）
  - `cfs_0.0.0-test.1_checksums.txt`
- 警告: `goreleaser/goreleaser-action@v6` が廃止予定の Node.js 20 向けだった。入力（`version` / `args`）が同じ v7（Node.js 24）に更新した。**v7 での実行は未確認**（次にタグを打ったときに確認できる）

## 未確認

- 成果物の実行ファイルを実際に動かしての確認（`cfs -v` でリビジョンが埋め込まれているか）。ダウンロードが必要なため行っていない
- zip が約 13MB と大きいのは、主に GCS（cloud.google.com/go/storage）の依存のため。[対応ストレージを絞る方針](2026-10-25-storage-scope-file-and-s3-only.md) で GCS を外せば小さくなる見込み

## 後片付け

確認用のリリース `v0.0.0-test.1` とタグは残してある。消すかどうかはオーナーの判断待ち。
