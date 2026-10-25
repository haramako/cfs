package cfs

import (
	"fmt"
	"path"
	"strings"
)

// アップロードの対象から外すファイルのパターン（.gitignore と似た書き方）
//
//	*.psd        どの階層でも、名前が合えば外す（フォルダなら中身ごと）
//	/build       アップロードするフォルダの直下だけ（"/" を含むパターンは、そのフォルダからのパス）
//	tmp/         フォルダだけ
//	docs/**/*.md "**" は 0 個以上のフォルダ
//	!keep.psd    それまでのパターンで外れたものを、対象に戻す（外したフォルダの中は戻せない）
//
// 後に書いたパターンほど優先する。.gitignore と違い、"#" で始まってもコメントではない

// DefaultExcludes は、いつも対象から外すもの。Exclude の先頭に書いたのと同じ扱いで、"!" で戻せる
var DefaultExcludes = []string{".*", "#*", "*~"}

type excludeRule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	segments []string // "/" で区切ったパターン。どの階層でも合うものは、先頭が "**"
}

// Excluder は、パスがアップロードの対象から外れるかを決める
type Excluder struct {
	rules []excludeRule
}

// NewExcluder は、DefaultExcludes に patterns を続けたルールを作る
func NewExcluder(patterns []string) (*Excluder, error) {
	e := &Excluder{}
	for _, p := range append(append([]string{}, DefaultExcludes...), patterns...) {
		rule, err := parseExcludeRule(p)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			e.rules = append(e.rules, *rule)
		}
	}
	return e, nil
}

func parseExcludeRule(p string) (*excludeRule, error) {
	rule := &excludeRule{pattern: p}
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
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", rule.pattern, err)
		}
	}
	return rule, nil
}

// Excluded は、アップロードするフォルダからの相対パス（"/" 区切り）が、対象から外れるかを返す
func (e *Excluder) Excluded(rel string, isDir bool) bool {
	name := strings.Split(rel, "/")
	excluded := false
	for _, r := range e.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if matchSegments(r.segments, name) {
			excluded = !r.negate
		}
	}
	return excluded
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
