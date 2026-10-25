# CFS - Content base File System

ゲームのアセットのような大量のファイルを、変更があった分だけ効率よくアップロード・ダウンロードするためのコマンドです。

- 中身のハッシュで保存するので、同じ中身のファイルはバージョンやプラットフォームをまたいで共有されます
- 変更があったファイルだけをアップロード・ダウンロードします
- 圧縮と暗号化を自動で行います
- 保存先は Amazon S3、Google Cloud Storage、ローカルのフォルダから選べます

## インストール

Go 1.26 以上が必要です。

```
go install github.com/haramako/cfs/cmd/cfs@latest
```

## はじめかた

作業するフォルダに `.cfsenv` を作り、アップロード先（キャビネット）を書きます。

```json
{
  "Cabinet": "s3://my-bucket"
}
```

フォルダをアップロードして、`v1` というタグを付けます。

```
cfs upload -t v1 assets
```

別の場所に、タグ `v1` の中身をダウンロードします。

```
cfs sync v1 downloaded
```

2回目以降は、変更があったファイルだけが転送されます。

S3 と GCS の認証は、AWS CLI や gcloud と同じ方法で設定します。ダウンロードは HTTP で行うので、ダウンロード元を HTTP で読めるようにしておく必要があります。詳しくは [ストレージ](docs/storage-backends.md) を見てください。

## ドキュメント

[docs/](docs/README.md) に、仕組み、設定、コマンド、ストレージ、制限などの説明があります。
