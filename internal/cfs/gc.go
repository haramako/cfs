package cfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ガーベージコレクション（GC）
//
// どのタグからも、保持期間内のマニフェストからも使われていないものを、キャビネットから消す。
//
//   - 残すもの: すべてのタグが指すマニフェストと、保持期間内に作られたマニフェスト（＝生きているマニフェスト）
//   - 消す候補のマニフェスト: 生きていないもの
//   - 消す候補のデータ: キャビネットに残っているどのマニフェストからも参照されず、保持期間より古いもの
//     （消す候補のマニフェストが参照するデータも、そのマニフェストが残っている間は消さない。
//     「マニフェストが先、中身は後」になるので、upload が前回のマニフェストの存在だけ確かめれば済む）
//   - 実際に消すのは、前回の GC でも候補だったものだけ（2 回連続で候補。アップロード途中との競合を防ぐ）
//   - 生きているマニフェストが 1 つでも読めなければ、何も消さずに中止する

const (
	cabinetConfigKey = "config.json"
	gcCandidatesKey  = "gc/candidates.json"
	gcLogPrefix      = "gc/log/"

	// DefaultRetention は、保持期間の既定値
	DefaultRetention = 30 * 24 * time.Hour
	// DefaultGCInterval は、候補にしてから消すまでに空ける最短の間隔
	DefaultGCInterval = time.Hour
)

// CabinetConfig は、キャビネットに置く設定（config.json）
type CabinetConfig struct {
	Retention string `json:"retention,omitempty"` // 保持期間（例: "30d"、"720h"）
}

// RetentionDuration は保持期間を返す
func (c *CabinetConfig) RetentionDuration() (time.Duration, error) {
	if c.Retention == "" {
		return DefaultRetention, nil
	}
	return ParseDuration(c.Retention)
}

// ParseDuration は、"30d" のような日数表記にも対応した期間を読む
func ParseDuration(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// LoadCabinetConfig は、キャビネットの設定を読む。なければ既定の設定を返す
func LoadCabinetConfig(f Fetcher) (*CabinetConfig, error) {
	cfg := &CabinetConfig{}
	data, err := fetchAll(f, cabinetConfigKey)
	if errors.Is(err, ErrNotFound) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", cabinetConfigKey, err)
	}
	return cfg, nil
}

// SaveCabinetConfig は、キャビネットの設定を書く
func SaveCabinetConfig(s Storage, cfg *CabinetConfig) error {
	if _, err := cfg.RetentionDuration(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return s.Put(cabinetConfigKey, append(data, '\n'), true)
}

// TagInfo は、タグの情報
type TagInfo struct {
	Name         string
	Hash         string // 指しているマニフェストのハッシュ
	LastModified time.Time
}

// ListTags は、キャビネットのタグの一覧を返す
func ListTags(s Storage, f Fetcher) ([]TagInfo, error) {
	objects, err := s.List("tag/")
	if err != nil {
		return nil, err
	}
	tags := make([]TagInfo, 0, len(objects))
	for _, o := range objects {
		data, err := fetchAll(f, o.Key)
		if err != nil {
			return nil, err
		}
		tags = append(tags, TagInfo{
			Name:         strings.TrimPrefix(o.Key, "tag/"),
			Hash:         strings.TrimSpace(string(data)),
			LastModified: o.LastModified,
		})
	}
	return tags, nil
}

// DeleteTags は、タグを消す（マニフェストや中身は、後の GC で消える）
func DeleteTags(s Storage, names []string) error {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		if err := ValidateTagName(name); err != nil {
			return err
		}
		keys = append(keys, tagKey(name))
	}
	return s.Delete(keys)
}

// GCOptions は、GC の設定
type GCOptions struct {
	DryRun    bool          // 何も消さず、何を消すかだけ調べる
	Retention time.Duration // 保持期間。0 ならキャビネットの設定（なければ既定値）
	Interval  time.Duration // 候補にしてから消すまでに空ける最短の間隔。0 なら既定値
	Now       time.Time     // 今の時刻（テスト用）。ゼロなら time.Now()
}

// GCResult は、GC の結果
type GCResult struct {
	Retention          time.Duration
	Tags               int
	Manifests          int
	LiveManifests      int
	Data               int
	CandidateManifests []string
	CandidateData      []string
	Deleted            []string // 消したもの（DryRun なら消すはずだったもの）
	DeletedBytes       int64
	SkippedData        bool // 読めない古いマニフェストがあり、今回はデータを候補にしなかった
}

type gcCandidates struct {
	Time time.Time `json:"time"`
	Keys []string  `json:"keys"`
}

// RunGC は、キャビネットの GC を行う
func RunGC(s Storage, f Fetcher, opt GCOptions) (*GCResult, error) {
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}
	interval := opt.Interval
	if interval == 0 {
		interval = DefaultGCInterval
	}
	retention := opt.Retention
	if retention == 0 {
		cfg, err := LoadCabinetConfig(f)
		if err != nil {
			return nil, err
		}
		if retention, err = cfg.RetentionDuration(); err != nil {
			return nil, err
		}
	}
	result := &GCResult{Retention: retention}

	keys, err := Option.KeyRing()
	if err != nil {
		return nil, err
	}

	// 1. タグが指すマニフェスト
	tags, err := ListTags(s, f)
	if err != nil {
		return nil, err
	}
	result.Tags = len(tags)
	tagged := map[string]bool{}
	for _, t := range tags {
		if !isHash(t.Hash) {
			return nil, fmt.Errorf("tag %s does not point to a manifest hash: %q", t.Name, t.Hash)
		}
		tagged[manifestKey(t.Hash)] = true
	}

	// 2. マニフェストを全部読み、参照されているデータを集める
	manifests, err := s.List("manifest/")
	if err != nil {
		return nil, err
	}
	result.Manifests = len(manifests)
	referenced := map[string]bool{}
	unreadable := false
	for _, m := range manifests {
		live := tagged[m.Key] || now.Sub(m.LastModified) < retention
		if live {
			result.LiveManifests++
		} else {
			result.CandidateManifests = append(result.CandidateManifests, m.Key)
		}

		hashes, err := readManifestData(f, m.Key, keys)
		if err != nil {
			if live {
				return nil, fmt.Errorf("cannot read live manifest %s, abort GC: %w", m.Key, err)
			}
			// 消す候補のマニフェストが読めない（古い鍵など）場合、その中身を守れないので、今回はデータを消さない
			if Verbose {
				fmt.Printf("cannot read old manifest %s: %v\n", m.Key, err)
			}
			unreadable = true
			continue
		}
		for _, h := range hashes {
			referenced[dataKey(h)] = true
		}
	}

	// 3. どのマニフェストからも参照されず、保持期間より古いデータ
	data, err := s.List("data/")
	if err != nil {
		return nil, err
	}
	result.Data = len(data)
	sizes := map[string]int64{}
	for _, m := range manifests {
		sizes[m.Key] = m.Size
	}
	for _, o := range data {
		sizes[o.Key] = o.Size
	}
	if unreadable {
		result.SkippedData = true
	} else {
		for _, o := range data {
			if !referenced[o.Key] && now.Sub(o.LastModified) >= retention {
				result.CandidateData = append(result.CandidateData, o.Key)
			}
		}
	}

	// 4. 前回も候補だったものだけを消す
	candidates := append(append([]string{}, result.CandidateManifests...), result.CandidateData...)
	prev, err := loadGCCandidates(f)
	if err != nil {
		return nil, err
	}
	if prev != nil && now.Sub(prev.Time) >= interval {
		wasCandidate := map[string]bool{}
		for _, k := range prev.Keys {
			wasCandidate[k] = true
		}
		for _, k := range candidates {
			if wasCandidate[k] {
				result.Deleted = append(result.Deleted, k)
				result.DeletedBytes += sizes[k]
			}
		}
	}

	if opt.DryRun {
		return result, nil
	}

	if len(result.Deleted) > 0 {
		if err := s.Delete(result.Deleted); err != nil {
			return nil, err
		}
		var log strings.Builder
		for _, k := range result.Deleted {
			fmt.Fprintf(&log, "%s\t%d\n", k, sizes[k])
		}
		if err := s.Put(gcLogPrefix+now.UTC().Format("20060102T150405Z")+".txt", []byte(log.String()), true); err != nil {
			return nil, err
		}
	}

	// 次回のために、今回の候補（消したものを除く）を残す
	deleted := map[string]bool{}
	for _, k := range result.Deleted {
		deleted[k] = true
	}
	next := gcCandidates{Time: now, Keys: []string{}}
	for _, k := range candidates {
		if !deleted[k] {
			next.Keys = append(next.Keys, k)
		}
	}
	sort.Strings(next.Keys)
	body, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if err := s.Put(gcCandidatesKey, body, true); err != nil {
		return nil, err
	}

	return result, nil
}

func readManifestData(f Fetcher, key string, keys *KeyRing) ([]string, error) {
	data, err := fetchAll(f, key)
	if err != nil {
		return nil, err
	}
	// マニフェストは DefaultContentAttribute で置かれるが、GC する側の設定と違っても読めるよう、
	// 暗号化（鍵 ID のヘッダー）と圧縮（zlib）をデータから判断する
	plain, err := decodeManifest(data, keys)
	if err != nil {
		return nil, err
	}
	m := NewManifest()
	if err := m.Parse(plain, nil); err != nil {
		return nil, err
	}
	return m.DataHashes(), nil
}

// decodeManifest は、属性が分からないマニフェストのデータを元に戻す
func decodeManifest(data []byte, keys *KeyRing) ([]byte, error) {
	for _, attr := range []ContentAttribute{NoContentAttribute, Compressed, Crypted, Compressed | Crypted} {
		plain, err := decode(data, keys, attr)
		if err == nil && strings.HasPrefix(string(plain), manifestHeader) {
			return plain, nil
		}
	}
	return nil, errors.New("cannot decode manifest (wrong encryption key?)")
}

func loadGCCandidates(f Fetcher) (*gcCandidates, error) {
	data, err := fetchAll(f, gcCandidatesKey)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := &gcCandidates{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", gcCandidatesKey, err)
	}
	return c, nil
}

// ValidateTagName は、タグ名に使える文字（A-Za-z0-9._-）だけかを確かめる
func ValidateTagName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid tag name %q", name)
	}
	for _, c := range name {
		if !(('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '.' || c == '_' || c == '-') {
			return fmt.Errorf("invalid tag name %q: only A-Za-z0-9._- can be used", name)
		}
	}
	return nil
}
