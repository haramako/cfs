# CFS - Content base File System -

`CFS`は大量のファイルを高速にアップロードやダウンロードを行うためのシステムです。

ゲームのアセットのアップロード＆ダウンロードに最適化され、そのために下記のような管理をおこなっています。

- 変更があったファイルだけをアップロードする
- 複数のバージョンやプラットフォーム別のファイルセットで同じ内容のファイルを共有して能率よく管理する
- 変更があったファイルだけをダウンロードして、最新の状態を効率よく保つことができる
- ファイルの暗号化/圧縮を透過的に行う


## 速度

約300MB / 2000個のファイルを初回アップロードで ?? 秒程度です。
２回目以降は更新がほとんどなければ、１秒程度でアップロードが終了します。

また、一度アップロードされていれば、他のホストの新規アップロードも高速になり、??秒程度です。

ダウンロードも、更新がなければ、32byteのハッシュ値の取得と比較だけで完了します。




## Getting Started

    $ cfs upload --tag test upload_files
    $ cfs sync --tag test downloaded_files


TODO: 追記する

## 設定ファイル

設定は、`.cfsenv`ファイルに設定します。

TODO: 設定ファイルの中身


## コマンドラインオプション

```
$ cfs -h
NAME:
   cfs - cfs client

USAGE:
   cfs [global options] command [command options] [arguments...]

VERSION:
   0.0.0

COMMANDS:
   upload	upload files to cabinet
   sync		sync from cabinet
   merge	merge buckets
   cat		fetch a data from url (for debug)
   ls		list files in bucket
   config	show current config
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --verbose, -V		verbose
   --config, -C ".cfsenv"	config file
   --cabinet, -c 		cabinet URL
   --help, -h			show help
   --version, -v		print the version
```

    $ cfs upload <対象のディレクトリ> ...



## アップロードされたファイルの構成

アップロードされたファイル大きく分けて`コンテンツデータ`と`メタデータ`のふたつに分類されます。

`コンテンツデータ`は内容のハッシュにより決まったパスに格納されます。

たとえば、ハッシュが`c59548c3c576228486a1f0037eb16a1b`だった場合、`/data/c5/9548c3c576228486a1f0037eb16a1b` という場所に保存されます。

このため、別のファイルだったとしても、同じハッシュ、つまり同じ内容のファイルは、１つのファイルとして扱われます。つまり、一度アップロードされていれば再度アップロードする必要もなく、１度ダウンロードされていれば再度ダウンロードする必要がありません。

これにより、能率的に差分をアップロード・ダウンロードすることができます。

また、特定のパスのファイルは、一度保存されると常に同じ内容で更新されることはないため、HTTPのproxyなどにより安全にキャッシュすることができます。



`メタデータ`は、`コンテンツデータ`と違い、コンテンツデータをあらわすハッシュが保存されています。

つまり、`メタデータ`は、安全にキャッシュすることはできないので、毎回取得する必要があります。


これらのファイルの構成は`git`の仕組みによく似ています。


## 暗号化/圧縮について

ファイルの暗号化/圧縮は透過的に行われます。

ファイルは、暗号化/圧縮されたままアップロードされ、ダウンロードされ、ストレージに保存されます。
その際、ファイルのハッシュは、暗号化/圧縮されたもののハッシュが使用されます。

CFSのクライアントライブラリのほうで、それらは自動で復号化/展開されるので、多くの場合は、使用者は暗号化されているかどうかを考える必要はありません。


暗号化/圧縮されたまま扱いたいアセットなどは、暗号化/圧縮をオフにすることが可能です。
多くの場合は、識別子で暗号化/圧縮を行うかどうかを制御するのが適切で、それらを`.cfsenv`で指定することができます。


## TODO

* CFS整備
 - タグ
 * ガーベージコレクト
 - S3対応(s3://bucket/dir)
 - GS対応(gs://bucket/dir)
 - SV対応(cfs://)
 * DL対応(http://)
 * 拡張子による暗号化などの変更対応
 * コマンドライン強化
  * other
    * cfs server
    * cfs init [-b gs://cfs]
  * upload(-b)
    * cfs upload [-t tag] hoge
  * bucket(-b)
    * cfs sync (tag|hash) out
    * cfs ls (tag|hash)
  * file(-b)
    * cfs cat [-o outfile] (tag|hash) file


- bucketファイルを自動で
* キャッシュクリアを追加
* statでいろいろ情報を確認
* info で現在のcfsenvを確認
