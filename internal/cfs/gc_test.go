package cfs

import (
	"os"
	"strings"
	"testing"
	"time"
)

// gcTest は、時刻を進めながら GC を試すための仕掛け
// gcTestConfig は、GC のテストでアップロードする設定。暗号化したマニフェストを GC が読めることも確かめる
var gcTestConfig = &Config{Compress: true, Recursive: true, EncryptKey: "gc-test-key-1234567890", NoCompress: []string{"*.raw"}, NoEncrypt: []string{"*.raw"}}

func gcTestKeys(t *testing.T) *KeyRing {
	keys, err := gcTestConfig.KeyRing()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

type gcTest struct {
	t   *testing.T
	s   Storage
	f   Fetcher
	now time.Time
}

// gcStorages は、GC を試すキャビネット。メモリ上と手元のフォルダの両方で同じことを確かめる
var gcStorages = []struct {
	name string
	open func(g *gcTest)
}{
	{"dummy", func(g *gcTest) {
		s, _ := NewDummyStorage("")
		s.now = func() time.Time { return g.now }
		g.s, g.f = s, s
	}},
	{"file", func(g *gcTest) {
		dir := g.t.TempDir()
		s, _ := NewFileStorage(dir)
		g.s = &timedFileStorage{FileStorage: s, now: func() time.Time { return g.now }}
		g.f = &fileFetcher{root: dir}
	}},
}

// timedFileStorage は、置いたファイルの更新日時をテストの時刻にする
type timedFileStorage struct {
	*FileStorage
	now func() time.Time
}

func (s *timedFileStorage) Put(key string, body []byte, overwrite bool) error {
	exists, err := s.Exists(key)
	if err != nil {
		return err
	}
	if err := s.FileStorage.Put(key, body, overwrite); err != nil {
		return err
	}
	if exists && !overwrite {
		return nil
	}
	t := s.now()
	return os.Chtimes(s.fullPath(key), t, t)
}

// runGCTest は、f をそれぞれのキャビネットで試す
func runGCTest(t *testing.T, f func(t *testing.T, g *gcTest)) {
	for _, st := range gcStorages {
		t.Run(st.name, func(t *testing.T) {
			g := &gcTest{t: t, now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
			st.open(g)
			f(t, g)
		})
	}
}

func (g *gcTest) upload(tag string, files map[string]string) *Manifest {
	return uploadWith(g.t, g.s, gcTestConfig, tag, files)
}

func (g *gcTest) gc(dryRun bool) *GCResult {
	g.t.Helper()
	r, err := RunGC(g.s, g.f, GCOptions{DryRun: dryRun, Retention: 10 * 24 * time.Hour, Interval: time.Hour, Now: g.now, Keys: gcTestKeys(g.t)})
	if err != nil {
		g.t.Fatal(err)
	}
	return r
}

func (g *gcTest) exists(key string) bool {
	ok, _ := g.s.Exists(key)
	return ok
}

func (g *gcTest) advance(d time.Duration) {
	g.now = g.now.Add(d)
}

func TestGCKeepsTaggedAndRecent(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		tagged := g.upload("v1", map[string]string{"a.txt": "tagged"})
		g.advance(20 * 24 * time.Hour)
		recent := g.upload("", map[string]string{"b.txt": "recent"})

		for i := 0; i < 3; i++ {
			g.advance(2 * time.Hour)
			if r := g.gc(false); len(r.Deleted) != 0 {
				t.Fatalf("must delete nothing but %v", r.Deleted)
			}
		}
		for _, m := range []*Manifest{tagged, recent} {
			if !g.exists(manifestKey(m.Hash)) {
				t.Errorf("manifest %s must be kept", m.Hash)
			}
			for _, h := range m.DataHashes() {
				if !g.exists(dataKey(h)) {
					t.Errorf("data %s must be kept", h)
				}
			}
		}
	})
}

func TestGCDeletesManifestFirstThenData(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		old := g.upload("", map[string]string{"a.txt": "old only"})
		oldData := dataKey(old.Contents["a.txt"].Hash)
		g.advance(20 * 24 * time.Hour) // 保持期間を過ぎる

		// 1 回目: 候補にするだけ
		r := g.gc(false)
		if len(r.Deleted) != 0 || len(r.CandidateManifests) != 1 {
			t.Fatalf("first run must only mark candidates: %+v", r)
		}
		// 古いマニフェストが残っているので、その中身はまだ候補にもしない
		if len(r.CandidateData) != 0 {
			t.Errorf("data of existing manifest must not be a candidate")
		}

		// 間隔が短いと消さない
		g.advance(10 * time.Minute)
		if r := g.gc(false); len(r.Deleted) != 0 {
			t.Errorf("must not delete within interval: %v", r.Deleted)
		}

		// 2 回連続で候補になったマニフェストを消す。中身はまだ残す
		g.advance(2 * time.Hour)
		r = g.gc(false)
		if len(r.Deleted) != 1 || r.Deleted[0] != manifestKey(old.Hash) {
			t.Fatalf("second run must delete the manifest: %v", r.Deleted)
		}
		if !g.exists(oldData) {
			t.Errorf("data must be kept until the next run after its manifest is deleted")
		}

		// 次の回で中身が候補になり、その次で消える
		g.advance(2 * time.Hour)
		if r := g.gc(false); len(r.Deleted) != 0 || len(r.CandidateData) != 1 {
			t.Fatalf("data must become a candidate: %+v", r)
		}
		g.advance(2 * time.Hour)
		if r := g.gc(false); len(r.Deleted) != 1 || r.Deleted[0] != oldData {
			t.Fatalf("data must be deleted: %v", r.Deleted)
		}
		if g.exists(oldData) {
			t.Errorf("data must be deleted")
		}

		// 消したものの記録が残る
		logs, _ := g.s.List(gcLogPrefix)
		if len(logs) != 2 {
			t.Errorf("gc log must be written for each deletion: %v", logs)
		}
	})
}

func TestGCKeepsSharedData(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		old := g.upload("", map[string]string{"shared.txt": "shared", "old.txt": "only old"})
		g.advance(20 * 24 * time.Hour)
		g.upload("v1", map[string]string{"shared.txt": "shared"}) // 同じ中身を使い回す

		for i := 0; i < 4; i++ {
			g.advance(2 * time.Hour)
			g.gc(false)
		}
		shared := dataKey(old.Contents["shared.txt"].Hash)
		only := dataKey(old.Contents["old.txt"].Hash)
		if !g.exists(shared) {
			t.Errorf("data shared with a tagged manifest must be kept")
		}
		if g.exists(only) {
			t.Errorf("data only used by the deleted manifest must be deleted")
		}
	})
}

func TestGCDryRun(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		g.upload("", map[string]string{"a.txt": "old"})
		g.advance(20 * 24 * time.Hour)
		g.gc(false)
		g.advance(2 * time.Hour)

		before, _ := g.s.List("")
		r := g.gc(true)
		after, _ := g.s.List("")
		if len(r.Deleted) != 1 {
			t.Errorf("dry run must report what would be deleted: %v", r.Deleted)
		}
		if len(before) != len(after) {
			t.Errorf("dry run must not change the cabinet")
		}
	})
}

func TestGCAbortsOnUnreadableLiveManifest(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		g.upload("v1", map[string]string{"a.txt": "a"})
		// 読めないマニフェストを、タグから指す
		broken := SumHash([]byte("broken"))
		g.s.Put(manifestKey(broken), []byte("broken"), true)
		g.s.Put(tagKey("broken"), []byte(broken), true)

		if _, err := RunGC(g.s, g.f, GCOptions{Retention: time.Hour, Now: g.now, Keys: gcTestKeys(t)}); err == nil || !strings.Contains(err.Error(), "abort") {
			t.Errorf("GC must abort when a live manifest is unreadable, but %v", err)
		}
	})
}

func TestGCSkipsDataWhenOldManifestUnreadable(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		g.upload("", map[string]string{"a.txt": "a"})
		broken := SumHash([]byte("broken"))
		g.s.Put(manifestKey(broken), []byte("broken"), true)
		g.advance(20 * 24 * time.Hour)

		r := g.gc(false)
		if !r.SkippedData || len(r.CandidateData) != 0 {
			t.Errorf("data must not be checked when an old manifest is unreadable: %+v", r)
		}
		if len(r.CandidateManifests) != 2 {
			t.Errorf("unreadable old manifest must still be a candidate: %v", r.CandidateManifests)
		}
	})
}

func TestGCRetentionFromCabinetConfig(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		if err := SaveCabinetConfig(g.s, &CabinetConfig{Retention: "2d"}); err != nil {
			t.Fatal(err)
		}
		r, err := RunGC(g.s, g.f, GCOptions{DryRun: true, Now: g.now})
		if err != nil {
			t.Fatal(err)
		}
		if r.Retention != 2*24*time.Hour {
			t.Errorf("retention must come from cabinet config but %v", r.Retention)
		}

		if err := SaveCabinetConfig(g.s, &CabinetConfig{Retention: "bad"}); err == nil {
			t.Errorf("invalid retention must be rejected")
		}
	})
}

func TestTags(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		m := g.upload("v1", map[string]string{"a.txt": "a"})
		g.upload("v2", map[string]string{"a.txt": "b"})

		tags, err := ListTags(g.s, g.f)
		if err != nil {
			t.Fatal(err)
		}
		if len(tags) != 2 || tags[0].Name != "v1" || tags[0].Hash != m.Hash {
			t.Errorf("invalid tags %+v", tags)
		}

		if err := DeleteTags(g.s, []string{"v1"}); err != nil {
			t.Fatal(err)
		}
		if g.exists(tagKey("v1")) || !g.exists(tagKey("v2")) {
			t.Errorf("only v1 must be deleted")
		}
		if err := DeleteTags(g.s, []string{"../config.json"}); err == nil {
			t.Errorf("invalid tag name must be rejected")
		}
	})
}

func TestGCAbortsWithoutKey(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		g.upload("v1", map[string]string{"a.txt": "a"})
		// 鍵がないと、暗号化したマニフェストが読めないので中止する
		if _, err := RunGC(g.s, g.f, GCOptions{Now: g.now}); err == nil || !strings.Contains(err.Error(), "abort") {
			t.Errorf("GC must abort without the key, but %v", err)
		}
	})
}

func TestGCKeepsEmptyData(t *testing.T) {
	runGCTest(t, func(t *testing.T, g *gcTest) {
		g.upload("v1", map[string]string{"empty.raw": ""})
		old := g.upload("", map[string]string{"old.raw": "", "old.txt": "old"})
		g.advance(20 * 24 * time.Hour)

		for i := 0; i < 4; i++ {
			g.advance(2 * time.Hour)
			g.gc(false)
		}
		// 0 バイトのデータも参照として数えるので、タグのあるマニフェストが使う間は消さない
		if !g.exists(dataKey(SumHash(nil))) {
			t.Errorf("0-byte data used by a tagged manifest must be kept")
		}
		if g.exists(dataKey(old.Contents["old.txt"].Hash)) {
			t.Errorf("data only used by the deleted manifest must be deleted")
		}
	})
}
