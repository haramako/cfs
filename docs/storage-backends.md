# ストレージ

アップロード先（キャビネット）は `.cfsenv` の `Cabinet` に URL で指定します。

| URL | ストレージ | 認証 |
|---|---|---|
| `file:///path/to/dir` | ローカルのフォルダ | 不要 |
| `s3://<バケット名>` | Amazon S3 | AWS の標準の認証（後述） |

今のところ、対応しているのはローカルのフォルダと Amazon S3 だけです。Google Cloud Storage（`gs://`）などは、今後対応する予定です。

`s3://` は、バケット名の後ろのパス（`s3://bucket/some/dir`）を今は無視します。キャビネットはバケットの直下に作られます。

## ダウンロード元

ダウンロードは、普通の HTTP(S) の GET で行います。ダウンロード元は次のように決まります。

1. `.cfsenv` に `Url` があれば、それを使います（CDN の URL を指定するなど）
2. なければ、`Cabinet` から自動で決めます

| `Cabinet` | 自動で決まるダウンロード元 |
|---|---|
| `file:///path` | `file:///path/` |
| `s3://<バケット名>` | `http://<バケット名>.s3-website-ap-northeast-1.amazonaws.com/` |

そのため、**S3 を使う場合は、ダウンロードするファイルを HTTP で読めるようにしておく必要があります。** 静的ウェブサイトホスティングを有効にして公開読み取りにするか、CloudFront などを前段に置いて `Url` に指定します。

キャビネットの `data/` の下は中身が変わらないので、CDN で長くキャッシュできます。`tag/` の下は上書きされるので、キャッシュしないか、短くしてください。

## Amazon S3

認証には、AWS の標準の仕組みを使います。AWS CLI と同じ方法で設定できます。

- 環境変数: `AWS_ACCESS_KEY_ID`、`AWS_SECRET_ACCESS_KEY`（必要なら `AWS_SESSION_TOKEN`）
- 設定ファイル: `~/.aws/credentials`、`~/.aws/config`。環境変数 `AWS_PROFILE` でプロファイルを選べます
- SSO: `aws sso login` でログインしたプロファイル
- EC2、ECS、EKS の上: そのマシンやタスクの IAM ロールが自動で使われます
- GitHub Actions: `aws-actions/configure-aws-credentials` の `role-to-assume`（OIDC）を使うと、長く使うキーを保存せずに済みます

手元の PC では、長く使うアクセスキーをファイルに置くより、SSO を使う方法をおすすめします。

アップロードには次の権限が必要です。

| 権限 | 用途 |
|---|---|
| `s3:ListBucket` | 同じ中身が既にあるかの確認 |
| `s3:PutObject` | ファイルとタグのアップロード |
| `s3:PutObjectAcl` | アップロード時に `bucket-owner-full-control` の ACL を付けるため（ACL が無効なバケットでは不要な場合があります） |

注意:

- **リージョンは `ap-northeast-1`（東京）に固定**です。`AWS_REGION` やプロファイルのリージョン設定は使われません
- 0 バイトのファイルはアップロードしません。ダウンロードするときは、ファイル一覧の情報から空のファイルを作ります

## ローカルのフォルダ

`file:///path/to/dir` で、ローカルや共有フォルダをキャビネットにできます。認証は不要です。試しに動かすときや、社内の共有フォルダで配るときに使えます。

Windows では、`file://` のキャビネットからのダウンロードが今は動きません。[制限と注意点](limitations.md) を見てください。
