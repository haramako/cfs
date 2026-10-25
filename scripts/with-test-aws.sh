#!/usr/bin/env bash
# 本物の S3 を使うテストやコマンドを、テスト専用の IAM ロールの一時的な認証情報で実行する。
#
#   scripts/with-test-aws.sh go test -count=1 ./internal/cfs
#   scripts/with-test-aws.sh cfs -c s3://<テスト用バケット> tag ls
#
# 設定はリポジトリに含めない .cfs-test.env から読む（docs/development.md を参照）。
# aws-sdk-go-v2 は、aws login のプロファイルを source_profile にしてロールを引き受けられないので、
# AWS CLI でロールの一時的な認証情報を取り出し、このコマンドの環境変数としてだけ渡す。
set -euo pipefail

cd "$(dirname "$0")/.."
if [ ! -f .cfs-test.env ]; then
  echo ".cfs-test.env がありません（docs/development.md を参照）" >&2
  exit 1
fi
set -a
. ./.cfs-test.env
set +a

eval "$(aws configure export-credentials --profile "$AWS_PROFILE" --format env)"
unset AWS_PROFILE

exec "$@"
