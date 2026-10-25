package cfs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/natefinch/atomic"
)

// FileStorage は、手元のフォルダをキャビネットにする
type FileStorage struct {
	CabinetPath string // キャビネットのフォルダ（絶対パス）
}

// NewFileStorage は、手元のパスのフォルダをキャビネットにする
func NewFileStorage(cabinetPath string) (*FileStorage, error) {
	abs, err := filepath.Abs(cabinetPath)
	if err != nil {
		return nil, err
	}
	return &FileStorage{CabinetPath: abs}, nil
}

func (s *FileStorage) fullPath(key string) string {
	return filepath.Join(s.CabinetPath, filepath.FromSlash(key))
}

func (s *FileStorage) Put(key string, body []byte, overwrite bool) error {
	file := s.fullPath(key)

	// 同じ中身は同じ場所になるので、既にあれば書かない
	if !overwrite {
		if _, err := os.Stat(file); err == nil {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(file), 0777); err != nil {
		return err
	}
	if err := atomic.WriteFile(file, bytes.NewReader(body)); err != nil {
		return err
	}

	if Verbose {
		fmt.Printf("uploading '%s'\n", key)
	}
	return nil
}

func (s *FileStorage) Exists(key string) (bool, error) {
	_, err := os.Stat(s.fullPath(key))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *FileStorage) List(prefix string) ([]ObjectInfo, error) {
	// prefix の手前のフォルダから辿り、key が prefix で始まるものを集める
	dir := s.CabinetPath
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		dir = s.fullPath(prefix[:i])
	}

	var result []ObjectInfo
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.CabinetPath, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		result = append(result, ObjectInfo{Key: key, Size: info.Size(), LastModified: info.ModTime()})
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, err
}

func (s *FileStorage) Delete(keys []string) error {
	for _, key := range keys {
		err := os.Remove(s.fullPath(key))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
