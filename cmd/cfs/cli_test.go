package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/haramako/cfs/internal/cfs"
)

// TestMain は、CFS_TEST_FILTER が設定されていれば、--filter-cmd に渡すフィルターとして動く
// （標準入力のパスのうち、その文字列を含むものだけを出力する）
func TestMain(m *testing.M) {
	if pattern := os.Getenv("CFS_TEST_FILTER"); pattern != "" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), pattern) {
				fmt.Println(scanner.Text())
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// cliTest は、一時フォルダを今のフォルダにして、cfs のコマンドをこのプロセスの中で動かす
type cliTest struct {
	t       *testing.T
	dir     string // 今のフォルダ
	cabinet string // 手元のフォルダのキャビネット（今のフォルダからの相対パス）
}

func newCLITest(t *testing.T) *cliTest {
	t.Helper()
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CFS_ENCRYPT_KEY", "cli-test-key-1234567890")
	t.Setenv("CFS_OLD_ENCRYPT_KEYS", "")
	t.Setenv("CFS_SIGNING_KEY", "")
	dir := t.TempDir()
	t.Chdir(dir)
	return &cliTest{t: t, dir: dir, cabinet: "cabinet"}
}

// tryRun は、cfs を引数で実行し、標準出力とエラー（panic を含む）を返す
func (c *cliTest) tryRun(args ...string) (out string, err error) {
	saved := *cfs.Option
	defer func() { *cfs.Option = saved }()

	r, w, perr := os.Pipe()
	if perr != nil {
		c.t.Fatal(perr)
	}
	stdout := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("panic: %v", p)
			}
		}()
		err = newApp().Run(context.Background(), append([]string{"cfs"}, args...))
	}()

	w.Close()
	os.Stdout = stdout
	return <-done, err
}

// run は、cfs を実行し、標準出力を返す。失敗したらテストを止める
func (c *cliTest) run(args ...string) string {
	c.t.Helper()
	out, err := c.tryRun(args...)
	if err != nil {
		c.t.Fatalf("cfs %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// write は、今のフォルダからの相対パスにファイルを書く
func (c *cliTest) write(files map[string]string) {
	c.t.Helper()
	for p, content := range files {
		full := filepath.Join(c.dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0777); err != nil {
			c.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0666); err != nil {
			c.t.Fatal(err)
		}
	}
}

// readTree は、フォルダの中のファイルを、スラッシュ区切りのパス → 中身にして返す
func (c *cliTest) readTree(dir string) map[string]string {
	c.t.Helper()
	result := map[string]string{}
	root := filepath.Join(c.dir, dir)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		result[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		c.t.Fatal(err)
	}
	return result
}

// lsPaths は、cfs ls の出力からパスだけを取り出す
func lsPaths(out string) string {
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			paths = append(paths, strings.SplitN(line, "\t", 2)[0])
		}
	}
	sort.Strings(paths)
	return strings.Join(paths, ",")
}

func equalTree(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("tree has %d files, want %d: %v", len(got), len(want), got)
	}
	for p, content := range want {
		if got[p] != content {
			t.Errorf("%s = %q, want %q", p, got[p], content)
		}
	}
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestCLIUploadAndDownload(t *testing.T) {
	c := newCLITest(t)
	src := map[string]string{"a.txt": "file a", "sub/b.txt": "file b", "empty.txt": "", "raw.raw": "raw file"}
	c.write(map[string]string{"src/a.txt": src["a.txt"], "src/sub/b.txt": src["sub/b.txt"], "src/empty.txt": "", "src/raw.raw": src["raw.raw"]})

	// キャビネットは .cfsenv の相対パス（.cfsenv のフォルダが基準）
	c.write(map[string]string{".cfsenv": `{"Cabinet": "` + c.cabinet + `"}`})
	if out := c.run("config"); !strings.Contains(out, filepath.Join(c.dir, c.cabinet)) {
		t.Errorf("cabinet must be resolved from .cfsenv: %s", out)
	}

	if out := c.run("cabinet", "init"); !strings.Contains(out, "init cabinet") {
		t.Errorf("unexpected init output: %s", out)
	}
	if out := c.run("cabinet", "config"); strings.TrimSpace(out) != "retention: 720h0m0s" {
		t.Errorf("default retention must be 30d: %s", out)
	}

	c.run("upload", "-t", "v1", "-o", "v1.hash", "src")
	hash, _ := os.ReadFile("v1.hash")
	if !hashPattern.Match(hash) {
		t.Fatalf("upload -o must write the manifest hash: %q", hash)
	}

	// ls はタグでもハッシュでも読める
	if got := lsPaths(c.run("ls", "v1")); got != "a.txt,empty.txt,raw.raw,sub/b.txt" {
		t.Errorf("ls v1 = %s", got)
	}
	if got := lsPaths(c.run("ls", string(hash))); got != "a.txt,empty.txt,raw.raw,sub/b.txt" {
		t.Errorf("ls <hash> = %s", got)
	}
	if out := c.run("ls", "--verify", "v1"); strings.Count(out, "\tok") != 4 {
		t.Errorf("all files must be ok: %s", out)
	}

	if out := c.run("cat", "v1", "sub/b.txt"); out != "file b" {
		t.Errorf("cat = %q", out)
	}

	c.run("sync", "v1", "out")
	equalTree(t, c.readTree("out"), src)

	// ファイルを消して上げ直すと、消したものはマニフェストからも消える
	os.Remove(filepath.Join(c.dir, "src", "raw.raw"))
	c.write(map[string]string{"src/a.txt": "file a v2", "src/c.txt": "file c"})
	c.run("upload", "-t", "v2", "src")
	if got := lsPaths(c.run("ls", "v2")); got != "a.txt,c.txt,empty.txt,sub/b.txt" {
		t.Errorf("ls v2 = %s", got)
	}
	if out := c.run("cat", "v2", "a.txt"); out != "file a v2" {
		t.Errorf("cat v2 a.txt = %q", out)
	}

	// --url で、キャビネットとは別の場所から読む（ここでは同じフォルダ）
	if out := c.run("--url", c.cabinet, "cat", "v1", "a.txt"); out != "file a" {
		t.Errorf("cat with --url = %q", out)
	}
}

func TestCLIMergeTagAndGC(t *testing.T) {
	c := newCLITest(t)
	c.write(map[string]string{"one/a.txt": "a1", "one/b.txt": "b1", "two/b.txt": "b2", "two/c.txt": "c2"})
	c.run("-c", c.cabinet, "cabinet", "init", "--retention", "7d")
	c.run("-c", c.cabinet, "upload", "-t", "one", "one")
	c.run("-c", c.cabinet, "upload", "-t", "two", "two")

	// 後に指定したものが優先される
	c.run("-c", c.cabinet, "merge", "-o", "merged.hash", "merged", "one", "two")
	if got := lsPaths(c.run("-c", c.cabinet, "ls", "merged")); got != "a.txt,b.txt,c.txt" {
		t.Errorf("ls merged = %s", got)
	}
	if out := c.run("-c", c.cabinet, "cat", "merged", "b.txt"); out != "b2" {
		t.Errorf("merged b.txt must come from the last location: %q", out)
	}
	if hash, _ := os.ReadFile("merged.hash"); !hashPattern.Match(hash) {
		t.Errorf("merge -o must write the manifest hash: %q", hash)
	}

	if got := lsPaths(c.run("-c", c.cabinet, "tag", "ls")); got != "merged,one,two" {
		t.Errorf("tag ls = %s", got)
	}
	c.run("-c", c.cabinet, "tag", "rm", "one")
	if got := lsPaths(c.run("-c", c.cabinet, "tag", "ls")); got != "merged,two" {
		t.Errorf("tag ls after rm = %s", got)
	}

	// 保持期間内なので、何も消さない
	out := c.run("-c", c.cabinet, "gc", "--dry-run")
	if !strings.Contains(out, "retention: 168h0m0s") || !strings.Contains(out, "would delete: 0") {
		t.Errorf("unexpected gc output: %s", out)
	}
	if out := c.run("-c", c.cabinet, "gc"); !strings.Contains(out, "deleted: 0") {
		t.Errorf("unexpected gc output: %s", out)
	}

	c.run("-c", c.cabinet, "cabinet", "config", "set", "retention", "2d")
	if out := c.run("-c", c.cabinet, "cabinet", "config"); strings.TrimSpace(out) != "retention: 48h0m0s" {
		t.Errorf("retention must be updated: %s", out)
	}
}

func TestCLISignedManifest(t *testing.T) {
	c := newCLITest(t)
	c.write(map[string]string{"src/a.txt": "signed"})
	c.run("-c", c.cabinet, "cabinet", "init")

	keys := c.run("keygen")
	private := regexp.MustCompile(`CFS_SIGNING_KEY=(\S+)`).FindStringSubmatch(keys)
	public := regexp.MustCompile(`"VerifyKeys": \["(\S+)"\]`).FindStringSubmatch(keys)
	if private == nil || public == nil {
		t.Fatalf("unexpected keygen output: %s", keys)
	}

	c.run("-c", c.cabinet, "upload", "-t", "unsigned", "src")
	t.Setenv("CFS_SIGNING_KEY", private[1])
	c.run("-c", c.cabinet, "upload", "-t", "signed", "src")

	// VerifyKeys があれば、署名のあるものだけ読める
	c.write(map[string]string{".cfsenv": `{"VerifyKeys": ["` + public[1] + `"]}`})
	if out := c.run("-c", c.cabinet, "cat", "signed", "a.txt"); out != "signed" {
		t.Errorf("signed manifest must be readable: %q", out)
	}
	if _, err := c.tryRun("-c", c.cabinet, "cat", "unsigned", "a.txt"); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("unsigned manifest must be rejected, but %v", err)
	}
}

func TestCLICabinetPolicy(t *testing.T) {
	c := newCLITest(t)
	out := c.run("-c", "s3://example-bucket", "cabinet", "policy", "upload")
	if !strings.Contains(out, "arn:aws:s3:::example-bucket/*") || !strings.Contains(out, "s3:PutObject") {
		t.Errorf("unexpected policy: %s", out)
	}
}

func TestCLIPackUnpackAndPatch(t *testing.T) {
	c := newCLITest(t)
	base := map[string]string{"a.txt": "pack a", "sub/b.txt": "pack b", "same.txt": "same"}
	c.write(map[string]string{"base/a.txt": base["a.txt"], "base/sub/b.txt": base["sub/b.txt"], "base/same.txt": "same"})
	c.write(map[string]string{"current/a.txt": "pack a v2", "current/sub/b.txt": "pack b", "current/same.txt": "same", "current/new.txt": "new"})

	c.run("pack", "base.tp", "base")
	if got := lsPaths(c.run("unpack", "base.tp")); got != "a.txt,same.txt,sub/b.txt" {
		t.Errorf("unpack list = %s", got)
	}
	c.run("unpack", "-o", "unpacked", "base.tp")
	equalTree(t, c.readTree("unpacked"), base)

	// パッチは、変わったものと増えたものだけ
	c.run("pack", "current.tp", "current")
	c.run("patch", "base.tp", "current.tp", "patch.tp")
	if got := lsPaths(c.run("unpack", "patch.tp")); got != "a.txt,new.txt" {
		t.Errorf("patch must contain changed and new files: %s", got)
	}

	// キャビネットのマニフェストからパックを作る
	c.run("-c", c.cabinet, "cabinet", "init")
	c.run("-c", c.cabinet, "upload", "-t", "v1", "base")
	c.run("-c", c.cabinet, "pack-manifest", "v1", "manifest.tp")
	c.run("unpack", "-o", "from-manifest", "manifest.tp")
	equalTree(t, c.readTree("from-manifest"), base)
}

func TestCLIFilterCmd(t *testing.T) {
	c := newCLITest(t)
	c.write(map[string]string{"src/keep-a.txt": "a", "src/drop-b.txt": "b", "src/sub/keep-c.txt": "c"})
	c.run("-c", c.cabinet, "cabinet", "init")
	c.run("-c", c.cabinet, "upload", "-t", "v1", "src")

	// このテストのバイナリ自身を、"keep" を含むパスだけを残すフィルターにする
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(exe, " ") {
		t.Skip("--filter-cmd splits the command by spaces")
	}
	t.Setenv("CFS_TEST_FILTER", "keep")
	want := map[string]string{"keep-a.txt": "a", "sub/keep-c.txt": "c"}

	c.run("-c", c.cabinet, "--filter-cmd", exe, "sync", "v1", "out")
	equalTree(t, c.readTree("out"), want)

	c.run("-c", c.cabinet, "--filter-cmd", exe, "merge", "filtered", "v1")
	if got := lsPaths(c.run("-c", c.cabinet, "ls", "filtered")); got != "keep-a.txt,sub/keep-c.txt" {
		t.Errorf("filtered merge = %s", got)
	}

	c.run("-c", c.cabinet, "--filter-cmd", exe, "pack-manifest", "v1", "filtered.tp")
	if got := lsPaths(c.run("unpack", "filtered.tp")); got != "keep-a.txt,sub/keep-c.txt" {
		t.Errorf("filtered pack-manifest = %s", got)
	}

	c.run("--filter-cmd", exe, "pack", "filtered-dir.tp", "src")
	if got := lsPaths(c.run("unpack", "filtered-dir.tp")); got != "keep-a.txt,sub/keep-c.txt" {
		t.Errorf("filtered pack = %s", got)
	}
}
