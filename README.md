# TODO

* CFS整備
 * タグ
 * ガーベージコレクト
 * S3対応(s3://bucket/dir)
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

