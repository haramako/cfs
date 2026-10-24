// golden.sh 用の静的ファイルサーバー（キャビネットを HTTP で配信する）
// 使い方: static-server <dir> <port>
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	log.Fatal(http.ListenAndServe("localhost:"+os.Args[2], http.FileServer(http.Dir(os.Args[1]))))
}
