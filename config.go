package cfs

import (
	"fmt"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
)

var globalCacheDir string
var globalDataCacheDir string

// CFSのキャッシュディレクトリを取得する
// ~/.cfs/cache がなければ作成してそれを返す
func GlobalCacheDir() string {
	if globalCacheDir != "" {
		return globalCacheDir
	}

	globalCacheDir := filepath.Join(HomeDir(), "cache")
	_, err := os.Stat(globalCacheDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(globalCacheDir, 0777)
		if err != nil {
			panic(err)
		}
	}
	return globalCacheDir
}

// CFSのデータキャッシュディレクトリを取得する
// ~/.cfs/data がなければ作成してそれを返す
func GlobalDataCacheDir() string {
	if globalDataCacheDir != "" {
		return globalDataCacheDir
	}

	globalDataCacheDir := filepath.Join(HomeDir(), "datacache")
	_, err := os.Stat(globalDataCacheDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(globalDataCacheDir, 0777)
		if err != nil {
			panic(err)
		}
	}
	return globalDataCacheDir
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
