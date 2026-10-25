package cfs

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// DummyStorage は、テスト用のメモリ上のキャビネット。書き込み（Storage）と読み込み（Fetcher）の両方を行う
type DummyStorage struct {
	CabinetPath string
	mutex       sync.Mutex // Put は複数のワーカーから並列に呼ばれる
	objects     map[string]dummyObject

	// now は、置いたものの更新日時に使う時刻（テストで差し替える）
	now func() time.Time

	onPut func(key string, body []byte, overwrite bool) error
}

type dummyObject struct {
	body         []byte
	lastModified time.Time
}

func NewDummyStorage(cabinetPath string) (*DummyStorage, error) {
	return &DummyStorage{
		CabinetPath: cabinetPath,
		objects:     make(map[string]dummyObject),
		now:         time.Now,
	}, nil
}

func (s *DummyStorage) Put(key string, body []byte, overwrite bool) error {
	if s.onPut != nil {
		if err := s.onPut(key, body, overwrite); err != nil {
			return err
		}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.objects[key]; ok && !overwrite {
		return nil
	}
	s.objects[key] = dummyObject{body: append([]byte(nil), body...), lastModified: s.now()}
	return nil
}

func (s *DummyStorage) Exists(key string) (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, ok := s.objects[key]
	return ok, nil
}

func (s *DummyStorage) List(prefix string) ([]ObjectInfo, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var result []ObjectInfo
	for key, o := range s.objects {
		if strings.HasPrefix(key, prefix) {
			result = append(result, ObjectInfo{Key: key, Size: int64(len(o.body)), LastModified: o.lastModified})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

func (s *DummyStorage) Delete(keys []string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, key := range keys {
		delete(s.objects, key)
	}
	return nil
}

func (s *DummyStorage) Open(key string) (io.ReadCloser, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	o, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return io.NopCloser(bytes.NewReader(o.body)), nil
}
