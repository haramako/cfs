# キャビネットを作るコマンド（cfs cabinet init）の計画

**状態: 計画（2026-10-25 オーナー指示）。未決事項あり（後述）。実装は未着手。**

## 背景

S3 のバケットを作るところから cfs でできるようにしたい。今は AWS のコンソールや CLI で別に作る必要があり、cfs に合った設定（非公開、ACL など）を手で揃える必要がある。

## 方針

`cfs cabinet init` で、`.cfsenv` の `Cabinet`（または `-c`）のキャビネットを作る。ローカルのフォルダと S3 の両方に対応する。

```
cfs cabinet init [--region リージョン] [--retention 30d] [--dry-run]
```

### S3（`s3://<バケット名>`）の場合

1. **バケットを作る**（CreateBucket）
   - リージョンは `--region` → `.cfsenv` の `S3Region` → AWS 標準の設定（`AWS_REGION`・プロファイル）→ `ap-northeast-1` の順
   - `us-east-1` 以外は `LocationConstraint` を付ける
   - 既に自分のバケットがある（`BucketAlreadyOwnedByYou`）なら作らずに次へ進む（何度実行してもよい）。他人のバケット名（`BucketAlreadyExists`）ならエラー
2. **非公開にする**: パブリックアクセスブロックを 4 項目すべて有効（PutPublicAccessBlock）。cfs は S3 から直接読むのが既定なので、公開は不要
3. **ACL を無効にする**: オブジェクト所有者を「バケット所有者の強制」（PutBucketOwnershipControls、`BucketOwnerEnforced`）。cfs のアップロードが付ける `bucket-owner-full-control` の ACL は、ACL 無効のバケットでも受け付けられる
4. **バージョニングは有効にしない**（GC で消しても容量が減らなくなるため）。暗号化は S3 の既定（SSE-S3）のまま
5. **キャビネットの設定 `config.json` を置く**: 保持期間（`--retention`、既定 30 日）と、**形式の版（`"format": 2`）**

### ローカルのフォルダの場合

フォルダを作り、`config.json` を置くだけ。

### `config.json` に形式の版を入れる

`init` で `"format": 2` を書いておき、読む側（`upload` / `sync` / `gc` など）は版が違う、または 2 より新しいキャビネットならエラーにする。`config.json` がない（`init` を使わずに作った）キャビネットは、今までどおり v2 として扱う。将来形式を変えたときに、古い cfs で壊さないため。

### `--dry-run`

何を作り、何を設定するかを表示するだけで、何も変えない。

## 必要な権限

`s3:CreateBucket`、`s3:PutBucketPublicAccessBlock`、`s3:PutBucketOwnershipControls`、`s3:PutObject`（`config.json`）。普段のアップロード用の権限とは分け、管理者が一度だけ実行する想定。

## テスト

- S3 の偽サーバーに、CreateBucket、PutPublicAccessBlock、PutBucketOwnershipControls を足して、送るリクエスト（リージョン、`LocationConstraint`、設定の中身）を確かめる
- 既に自分のバケットがある場合に続行し、他人のバケット名ならエラーになること
- `--dry-run` で何も送らないこと
- ローカルのフォルダで、フォルダと `config.json` ができること
- `config.json` の版が違うキャビネットを読もうとするとエラーになること

## 採らなかった案・範囲外

- **公開・CloudFront の設定**: 認証情報を持たせられない配布先に向けて公開する場合の CloudFront（OAC）の設定は、AWS 側の構成が大きいので範囲外。docs に手順の概要を書くにとどめる
- **バケットの削除**（`cabinet destroy`）: 取り返しがつかないので作らない

## 未決事項（オーナーに確認する）

1. コマンド名は `cfs cabinet init` でよいか
2. 非公開（パブリックアクセスブロック有効）と ACL 無効を、選べないようにしてよいか（公開したい場合は CloudFront を使う前提）
3. あわせて、用途ごと（アップロード用・ダウンロード用・GC 用）の IAM ポリシーの JSON を表示するコマンド（例: `cfs cabinet policy upload`）も作るか

## 追記（2026-10-25）: 実装した

オーナーが未決事項 1〜3 をすべて了承（コマンド名 `cfs cabinet init`、非公開と ACL 無効は固定、IAM ポリシーを表示するコマンドも作る）。計画どおり実装した。

- `cfs cabinet init [--region] [--retention] [--dry-run]`（`internal/cfs/cabinet_init.go`）。S3 はバケット作成 → パブリックアクセスブロック → ACL 無効 → `config.json`。ローカルはフォルダ作成 → `config.json`。既存の保持期間は `--retention` を指定しない限り残す
- `cfs cabinet policy upload|download|gc|admin`（`IAMPolicy`）。download は `s3:GetObject` だけ
- `config.json` に `format: 2`。`format` が 2 以外のキャビネットは、`upload`・ダウンロード（`LoadManifest`）・`gc`・`cabinet config set`・`cabinet init` で拒否する。書かれていなければ v2 として扱う
- S3 の偽サーバーに CreateBucket・PutPublicAccessBlock・PutBucketOwnershipControls を足してテストした。実際の S3 ではまだ試していない
