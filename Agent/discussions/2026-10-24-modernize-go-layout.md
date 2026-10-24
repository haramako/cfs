# Go モジュール構成と依存の現代化

## 背景

2021-06 以降休眠していた cfs を、現代の標準的な Go の構成にしたいという相談。「現状の挙動をなるべく維持したまま」が条件。

## 相談したこと・検討

- 既存の構成（ルートがライブラリ、`cmd/cfs`、`pack/`）は公式レイアウトにほぼ沿っており、問題はモジュール周り（`local.package/cfs` の `replace`、`go 1.12`、`ioutil`、CircleCI/goreleaser の陳腐化）と判断
- `pkg/` は作らない（公式が推奨していない）
- ルートパッケージを公開のまま残すか `internal/` に移すか → オーナーが **CLI 専用でよい** と判断し `internal/` に移動
- 段階: (1) 構成 (2) CI/リリース (3) 依存更新。(3) は挙動が変わりうるので別コミット

## 決定事項

- `internal/cfs`, `internal/pack` に移動、import は `github.com/haramako/cfs/internal/...`、`go 1.26`
- CircleCI を廃止して GitHub Actions（CI: ubuntu/windows で vet+test、Release: `v*` タグで goreleaser v2）
- 依存: goamz → aws-sdk-go-v2、google api storage/v1 → cloud.google.com/go/storage、urfave/cli v1 → v3、go-yaml → go.yaml.in/yaml/v3
- 既知の不具合（GCS のエラー握りつぶし等）は **挙動維持を優先してそのまま移植**。修正は別作業
- 挙動確認は `Agent/scripts/behavior-golden/` のゴールデンテストと、S3/GCS の偽サーバーテストで行う

## 結果

- ブランチ `modernize-layout` に 3 コミット（6d80b37, 8a64412, 20d4f1d）
- ゴールデンテストの差分は CLI のヘルプ表示の書式と、不正フラグのエラーが標準エラーに出る点のみ
- 意図した挙動差: S3 の認証が SDK 標準チェーンになり、認証情報がなくても `NewS3Storage` 自体は失敗しない（`Url` 未指定時のダウンロード URL 計算でも認証不要になった）。CLI はグローバルオプションをサブコマンドの後ろにも書けるようになった
- 未検証: goreleaser の設定チェック（`goreleaser check` の実行が許可されなかった）、GitHub Actions の実行、実 S3/GCS への接続
