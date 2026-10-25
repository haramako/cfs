package cfs

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/natefinch/atomic"
)

// FileStorage は、手元のフォルダをキャビネットにする
type FileStorage struct {
	CabinetPath string // キャビネットのフォルダ（絶対パス）
	Log         Logger

	mutex sync.Mutex
	locks map[string]*sync.Mutex
}

func (s *FileStorage) lockKey(key string) func() {
	s.mutex.Lock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	m, ok := s.locks[key]
	if !ok {
		m = &sync.Mutex{}
		s.locks[key] = m
	}
	s.mutex.Unlock()
	m.Lock()
	return m.Unlock
}

// NewFileStorage は、手元のパスのフォルダをキャビネットにする
func NewFileStorage(cabinetPath string) (*FileStorage, error) {
	return newFileStorage(cabinetPath, nil)
}

func newFileStorage(cabinetPath string, log Logger) (*FileStorage, error) {
	abs, err := filepath.Abs(cabinetPath)
	if err != nil {
		return nil, err
	}
	return &FileStorage{CabinetPath: abs, Log: log}, nil
}

func (s *FileStorage) fullPath(key string) string {
	return filepath.Join(s.CabinetPath, filepath.FromSlash(key))
}

func (s *FileStorage) Put(key string, body []byte, overwrite bool) error {
	// 同じプロセスの中では、同じ場所への書き込みを順番にする
	unlock := s.lockKey(key)
	defer unlock()

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
		// 同じ場所に別のプロセスが同時に書き、相手が先に置いた場合（Windows では置き換えに失敗する）。
		// overwrite しない場所（data/ や manifest/）は中身で決まるので、置かれていれば同じ中身
		if !overwrite {
			if _, serr := os.Stat(file); serr == nil {
				return nil
			}
		}
		return err
	}

	s.Log.Printf("uploading '%s'\n", key)
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
