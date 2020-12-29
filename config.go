package cfs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-yaml/yaml"
)

type SettingInfo struct {
	HomeRoot string `yaml:"HomeRoot"`
}

var globalCacheDir string
var globalDataCacheDir string
var globalSettingPath string
var Setting *SettingInfo

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

// CFSの設定ファイルのパスを返す
func GlobalSettingPath() string {
	if globalSettingPath != "" {
		return globalSettingPath
	}
	homeRoot, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		panic("cannot get home dir")
	}
	globalSettingPath := filepath.Join(homeRoot, ".cfs_setting")
	return globalSettingPath
}

// ユーザーのホームディレクトリを取得する
func HomeDir() string {
	InitDefaultSetting() // TODO: remove from here, and load automatically

	home := filepath.Join(Setting.HomeRoot, ".cfs")
	_, err := os.Stat(home)
	if os.IsNotExist(err) {
		err := os.MkdirAll(home, 0777)
		if err != nil {
			panic(err)
		}
	}

	return home
}

// Settingに初期設定を行う
func InitDefaultSetting() {
	homeRoot, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		panic("cannot get home dir")
	}
	Setting = &SettingInfo{
		HomeRoot: homeRoot,
	}

	err = Setting.Load()
	if err != nil {
		fmt.Println(err)
	}
}

// .cfs_settingファイルを読み込んで設定を反映する
func (s *SettingInfo) Load() error {

	_, err := os.Stat(GlobalSettingPath())
	if os.IsNotExist(err) {
		return nil
	}

	f, err := os.Open(GlobalSettingPath())
	if err != nil {
		return err
	}
	defer f.Close()
	err = yaml.NewDecoder(f).Decode(&s)
	return err
}
