package cfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// RemoveUnlisted は、dir の中で、マニフェストにないファイルを消し、空になったフォルダも消す（dir 自身は残す）。
// 消したもの（dir からの "/" 区切りの相対パス）を返す。sync の --delete で、Sync の後に使う
func (d *Downloader) RemoveUnlisted(m *Manifest, dir string) ([]string, error) {
	listed := map[string]bool{}
	for path := range m.Contents {
		listed[pathKey(path)] = true
	}

	var removed []string
	var dirs []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		if listed[pathKey(rel)] {
			return nil
		}
		d.log.Printf("delete %s\n", rel)
		if err := os.Remove(p); err != nil {
			return err
		}
		removed = append(removed, rel)
		return nil
	})
	if err != nil {
		return removed, err
	}

	// 空になったフォルダを、深いものから消す
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, p := range dirs {
		entries, err := os.ReadDir(p)
		if err != nil {
			return removed, err
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(p); err != nil {
			return removed, err
		}
		rel, _ := filepath.Rel(dir, p)
		removed = append(removed, filepath.ToSlash(rel)+"/")
	}
	return removed, nil
}

// pathKey は、手元のファイルとマニフェストのパスを比べるための形にする。
// マニフェストのパスは NFC。Windows と macOS のファイルシステムは、ふつう大文字と小文字を区別しないので、そろえる
func pathKey(p string) string {
	p = norm.NFC.String(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}
