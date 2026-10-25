package cfs

import (
	"testing"
)

func TestExcluder(t *testing.T) {
	e, err := NewExcluder([]string{"*.meta", "/build", "tmp/", "docs/**/*.md", "Assets/*.psd", "!keep.meta", "#*", "!.htaccess"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path     string
		isDir    bool
		excluded bool
	}{
		// いつも外すもの（フォルダにも効く）
		{".DS_Store", false, true},
		{"sub/.gitkeep", false, true},
		{".hidden", true, true},
		{"#autosave#", false, true},
		{"a.txt~", false, true},
		{"a.txt", false, false},
		// "!" で戻せる
		{".htaccess", false, false},
		// "/" を含まないパターンは、どの階層でも
		{"a.meta", false, true},
		{"x/y/a.meta", false, true},
		{"keep.meta", false, false},
		{"x/keep.meta", false, false},
		// 先頭の "/" は、アップロードするフォルダの直下だけ
		{"build", true, true},
		{"build", false, true},
		{"x/build", true, false},
		// 末尾の "/" はフォルダだけ
		{"tmp", true, true},
		{"x/tmp", true, true},
		{"tmp", false, false},
		// "**" は 0 個以上のフォルダ
		{"docs/a.md", false, true},
		{"docs/x/y/a.md", false, true},
		{"docs/a.txt", false, false},
		{"x/docs/a.md", false, false},
		// 途中に "/" があれば、アップロードするフォルダからのパス
		{"Assets/a.psd", false, true},
		{"Assets/x/a.psd", false, false},
	} {
		if got := e.Match(tc.path, tc.isDir); got != tc.excluded {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", tc.path, tc.isDir, got, tc.excluded)
		}
	}
}

func TestExcluderTrailingDoubleStar(t *testing.T) {
	e, _ := NewExcluder([]string{"logs/**"})
	if e.Match("logs", true) {
		t.Errorf("logs/** must not match the folder itself")
	}
	if !e.Match("logs/a.txt", false) || !e.Match("logs/x/a.txt", false) {
		t.Errorf("logs/** must match everything inside")
	}
}

func TestExcluderInvalidPattern(t *testing.T) {
	if _, err := NewExcluder([]string{"[abc"}); err == nil {
		t.Errorf("invalid pattern must be an error")
	}
	if e, err := NewExcluder([]string{"", "  ", "!"}); err != nil || e.Match("a.txt", false) {
		t.Errorf("empty patterns must be ignored: %v", err)
	}
}

func TestPathPatternsMatchFile(t *testing.T) {
	pp, err := NewPathPatterns([]string{"*.ab", "Movies/", "!Movies/keep.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for path, expect := range map[string]bool{
		"a.ab":            true,
		"x/y/a.ab":        true,
		"a.txt":           false,
		"Movies/a.txt":    true, // 親のフォルダが選ばれていれば、中のファイルも選ばれる
		"x/Movies/a.txt":  true,
		"Movies/keep.txt": true, // 選ばれたフォルダの中は戻せない（.gitignore と同じ）
		"MoviesX/a.txt":   false,
		".DS_Store":       false, // DefaultExcludes は NewExcluder だけで使う
	} {
		if got := pp.MatchFile(path); got != expect {
			t.Errorf("MatchFile(%q) = %v, want %v", path, got, expect)
		}
	}
}
