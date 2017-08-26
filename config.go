package cfs

import (
	"fmt"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

var globalCacheDir string

// CFSのキャッシュディレクトリを取得する
// ~/.cfs/cache がなければ作成してそれを返す
func GlobalCacheDir() string {
	if globalCacheDir != "" {
		return globalCacheDir
	}

	globalCacheDir := filepath.Join(HomeDir(), "cache")
	_, err := os.Stat(globalCacheDir)
	if !os.IsExist(err) {
		err := os.MkdirAll(globalCacheDir, 0777)
		if err != nil {
			panic(err)
		}
	}
	return globalCacheDir
}

// ユーザーのホームディレクトリを取得する
func HomeDir() string {
	homeRoot, err := homedir.Dir()
	if err != nil {
		fmt.Println(err)
		panic("cannot get home dir")
	}

	home := filepath.Join(homeRoot, ".cfs")

	_, err = os.Stat(home)
	if !os.IsExist(err) {
		err := os.MkdirAll(home, 0777)
		if err != nil {
			panic(err)
		}
	}

	return home
}
