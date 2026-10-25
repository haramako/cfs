package cfs

import (
	"fmt"
	"path"
	"strings"
)

// ファイルを選ぶパターン（.gitignore と似た書き方）。.cfsenv の Exclude / NoEncrypt / NoCompress で使う
//
//	*.psd        どの階層でも、名前が合えば選ぶ（フォルダなら中身ごと）
//	/build       アップロードするフォルダの直下だけ（"/" を含むパターンは、そのフォルダからのパス）
//	tmp/         フォルダだけ
//	docs/**/*.md "**" は 0 個以上のフォルダ
//	!keep.psd    それまでのパターンで選んだものを、選ばないことにする（選んだフォルダの中は戻せない）
//
// 後に書いたパターンほど優先する。.gitignore と違い、"#" で始まってもコメントではない

// DefaultExcludes は、いつもアップロードの対象から外すもの。Exclude の先頭に書いたのと同じ扱いで、"!" で戻せる
var DefaultExcludes = []string{".*", "#*", "*~"}

type pathRule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	segments []string // "/" で区切ったパターン。どの階層でも合うものは、先頭が "**"
}

// PathPatterns は、パスがパターンで選ばれるかを決める
type PathPatterns struct {
	rules []pathRule
}

// NewPathPatterns は、パターンの一覧からルールを作る
func NewPathPatterns(patterns []string) (*PathPatterns, error) {
	pp := &PathPatterns{}
	for _, p := range patterns {
		rule, err := parsePathRule(p)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			pp.rules = append(pp.rules, *rule)
		}
	}
	return pp, nil
}

// NewExcluder は、DefaultExcludes に patterns を続けた、アップロードの対象から外すルールを作る
func NewExcluder(patterns []string) (*PathPatterns, error) {
	return NewPathPatterns(append(append([]string{}, DefaultExcludes...), patterns...))
}

func parsePathRule(p string) (*pathRule, error) {
	rule := &pathRule{pattern: p}
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "!") {
		rule.negate = true
		p = p[1:]
	}
	if strings.HasSuffix(p, "/") {
		rule.dirOnly = true
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		return nil, nil
	}
	// "/" を含むなら、アップロードするフォルダからのパス。含まなければ、どの階層の名前にも合う
	anchored := strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")
	rule.segments = strings.Split(p, "/")
	if !anchored {
		rule.segments = append([]string{"**"}, rule.segments...)
	}
	for _, s := range rule.segments {
		if _, err := path.Match(s, ""); err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", rule.pattern, err)
		}
	}
	return rule, nil
}

// Match は、アップロードするフォルダからの相対パス（"/" 区切り）が選ばれるかを返す。親のフォルダは見ない
func (pp *PathPatterns) Match(rel string, isDir bool) bool {
	name := strings.Split(rel, "/")
	matched := false
	for _, r := range pp.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if matchSegments(r.segments, name) {
			matched = !r.negate
		}
	}
	return matched
}

// MatchFile は、ファイルが選ばれるかを返す。親のフォルダが選ばれていれば、中のファイルも選ばれる（.gitignore と同じ）
func (pp *PathPatterns) MatchFile(rel string) bool {
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		if pp.Match(strings.Join(parts[:i], "/"), true) {
			return true
		}
	}
	return pp.Match(rel, false)
}

// matchSegments は、パターンの区切りとパスの区切りを先頭から合わせる。"**" は 0 個以上の区切りに合う
func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return len(name) > 0 // 末尾の "**" は、中のものすべて
		}
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	if ok, _ := path.Match(pattern[0], name[0]); !ok {
		return false
	}
	return matchSegments(pattern[1:], name[1:])
}
