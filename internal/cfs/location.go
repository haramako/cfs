package cfs

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// キャビネットやダウンロード元は、URL（"s3://bucket" や "file:///C:/dir" など）か、
// 手元のパス（"./cabinet" や "C:\dir" など）で指定できる。
// "://" を含むものを URL、それ以外をパスとして扱う。

// IsURL は、場所の指定が URL かどうかを返す
func IsURL(location string) bool {
	return strings.Contains(location, "://")
}

// ResolveLocation は、場所の指定を正規化する。
// URL はそのまま返し、パスは base（絶対パスのフォルダ）を基準にした絶対パスにする。
func ResolveLocation(location string, base string) (string, error) {
	if location == "" || IsURL(location) {
		return location, nil
	}
	return resolveLocalPath(location, base)
}

func resolveLocalPath(p string, base string) (string, error) {
	if isWindows() {
		// "C:hoge" は、C ドライブの「今のフォルダ」からの相対パスになり分かりにくいので受け付けない
		if isDriveRelative(p) {
			return "", fmt.Errorf("drive-relative path %q is not supported, use an absolute path like %q or a relative path", p, p[:2]+`\`+p[2:])
		}
		if filepath.IsAbs(p) {
			return filepath.Clean(p), nil
		}
		// "\hoge" や "/hoge" は、基準のフォルダのドライブのルートからのパス
		if strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "/") {
			return filepath.Clean(filepath.VolumeName(base) + p), nil
		}
		return filepath.Join(base, p), nil
	}

	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Join(base, p), nil
}

func isDriveLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// isDriveRelative は "C:" や "C:hoge" の形（ドライブ名の直後に区切り文字がない）かどうかを返す
func isDriveRelative(p string) bool {
	return len(p) >= 2 && isDriveLetter(p[0]) && p[1] == ':' && (len(p) == 2 || (p[2] != '\\' && p[2] != '/'))
}

// FileURLToPath は "file://" の URL を手元のパスに変換する
func FileURLToPath(u *url.URL) (string, error) {
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		if isWindows() {
			// file://server/share/dir は \\server\share\dir
			return `\\` + u.Host + filepath.FromSlash(p), nil
		}
		return "", fmt.Errorf("file URL with host %q is not supported: %v (use a path like ./dir for a relative path)", u.Host, u)
	}
	if isWindows() {
		// file:///C:/dir の Path は "/C:/dir" なので、先頭の "/" を落とす
		if len(p) >= 3 && p[0] == '/' && isDriveLetter(p[1]) && p[2] == ':' {
			p = p[1:]
		}
	}
	if p == "" {
		return "", fmt.Errorf("file URL without path: %v", u)
	}
	return filepath.Clean(filepath.FromSlash(p)), nil
}

// PathToFileURL は手元の絶対パスを "file://" の URL に変換する
func PathToFileURL(p string) *url.URL {
	if isWindows() && strings.HasPrefix(p, `\\`) {
		// \\server\share\dir は file://server/share/dir
		rest := filepath.ToSlash(p[2:])
		host, path, _ := strings.Cut(rest, "/")
		return &url.URL{Scheme: "file", Host: host, Path: "/" + path}
	}
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return &url.URL{Scheme: "file", Path: p}
}
