#!/usr/bin/env bash
# cfs バイナリの外部から見える挙動を記録する。
# 使い方: golden.sh <cfs binary> <result file> [work dir]
# 2つのバイナリの結果ファイルを diff して挙動差分を確認する。
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CFS="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
RESULT="$2"
WORK="${3:-${TMPDIR:-/tmp}/cfs-golden-work}"

rm -rf "$WORK"
mkdir -p "$WORK"
cd "$WORK" || exit 1

# ~/.cfs を汚さないようにホームを差し替える（Windows は USERPROFILE、他は HOME）
export USERPROFILE="$(cygpath -w "$WORK/home" 2>/dev/null || echo "$WORK/home")"
export HOME="$WORK/home"
mkdir -p "$WORK/home"

WORK_URL="$(cygpath -m "$WORK" 2>/dev/null || echo "$WORK")"
WORK_URL="${WORK_URL#/}"
# ダウンロードは静的サーバー経由（Windows では file:// のダウンロードが元から動かないため）
cat > .cfsenv <<EOF
{
  "Cabinet": "file:///$WORK_URL/cabinet",
  "Url": "http://localhost:18764/",
  "EncryptKey": "12345678901234567890123456789012",
  "EncryptIv": "1234567890123456"
}
EOF
mkdir -p cabinet

(cd "$SCRIPT_DIR/static-server" && go build -o "$WORK/static-server.exe" .) || exit 1
"$WORK/static-server.exe" cabinet 18764 >/dev/null 2>&1 &
STATIC_PID=$!
trap 'kill $STATIC_PID 2>/dev/null' EXIT

# fixture
mkdir -p src/sub/deep src/.hidden
printf 'hello\n' > src/a.txt
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n' > src/b.txt
printf 'raw bundle data' > src/bundle.ab
: > src/empty.bin
printf 'sub file\n' > src/sub/c.txt
printf 'deep file\n' > src/sub/deep/d.txt
printf 'unicode\n' > "src/sub/日本語.txt"
printf 'meta\n' > src/a.txt.meta
printf 'dot\n' > src/.dotfile
printf 'hidden\n' > src/.hidden/x.txt
printf 'backup\n' > src/b.txt~
grep -v '"Url"' .cfsenv > nourl.cfsenv
find src -exec touch -d '2020-01-02T03:04:05Z' {} +

exec 3>"$RESULT"
run() {
  echo "### $*" >&3
  "$CFS" "$@" >out.tmp 2>err.tmp
  echo "exit=$?" >&3
  cat out.tmp >&3
  # panic のスタックトレースはパスが変わるので先頭行だけ比較する
  [ -s err.tmp ] && { echo "--- stderr(head)" >&3; head -1 err.tmp >&3; }
}
dump() {
  echo "### dump $1" >&3
  (cd "$1" && find . -type f | LC_ALL=C sort | while read -r f; do echo "$(md5sum < "$f" | cut -c1-32) $f"; done) >&3
}

run -h
run help upload
run help sync
run help unpack
run config
run setting

run upload -t v1 -o hash1.txt src
echo "### hash1" >&3; cat hash1.txt >&3; echo >&3
dump cabinet
dump home/.cfs
run ls v1
run ls "$(cat hash1.txt)"
run ls --verify v1
run cat v1 a.txt
run cat v1 sub/日本語.txt
run cat v1 nothing.txt
# Url 未指定（cabinet から導出した file:// でダウンロード）
run -C nourl.cfsenv config
run -C nourl.cfsenv ls v1
# ダウンロード元を手元のパスで指定
run --url cabinet ls v1
run --url ./cabinet cat v1 a.txt
run --url C:hoge ls v1

# 変更なしで再アップロード（キャッシュ利用）
run -V upload -t v1 src

# 変更して v2
printf 'hello v2\n' > src/a.txt
touch -d '2021-01-02T03:04:05Z' src/a.txt
rm src/b.txt
printf 'new\n' > src/new.txt
touch -d '2021-01-02T03:04:05Z' src/new.txt
run upload -t v2 src
run ls v2
run -f upload -t v2f src
run ls v2f

# 引数の後ろにフラグ
run upload src -t v3
run ls v3

run merge -o merged.txt merged v1 v2
echo "### merged" >&3; cat merged.txt >&3; echo >&3
run ls merged

run sync v2 out-sync
dump out-sync
run --filter-cmd "grep sub" sync v1 out-filter
dump out-filter
run sync v2
run sync nosuchtag out-x

run pack p1.tp src
run unpack p1.tp
run unpack p1.tp -o out-unpack
dump out-unpack
run -o out-unpack2 unpack p1.tp
run pack-manifest v1 pb1.tp
run unpack pb1.tp
run patch pb1.tp p1.tp patch.tp
run unpack patch.tp
run --filter-cmd "grep deep" pack pf.tp src
run unpack pf.tp
echo "### pack md5" >&3
md5sum p1.tp pb1.tp patch.tp pf.tp >&3

dump cabinet

# タグ、キャビネットの設定、GC（更新日時は実行のたびに変わるので除く）
echo "### tag ls" >&3
"$CFS" tag ls 2>&1 | cut -f1,2 >&3
run tag rm v2f
run tag rm feature/foo
run cabinet config
run cabinet config set retention 1d
run cabinet config
run gc --dry-run
run gc
echo "### keygen lines" >&3
"$CFS" keygen | grep -c '^CFS_SIGNING_KEY=\|"VerifyKeys"' >&3
run upload -t feature/foo src

# serve（HTTP サーバー）
"$CFS" serve --listen 127.0.0.1:18765 >http.log 2>&1 &
HTTP_PID=$!
for i in $(seq 1 50); do curl -s -o /dev/null http://localhost:18765/v1/a.txt && break; sleep 0.2; done
for p in /v1/a.txt /v1/sub/ /v1/sub "/v1/sub/deep/d.txt"; do
  echo "### http $p" >&3
  curl -s -D - "http://localhost:18765$p" | tr -d '\r' | grep -v '^Date:' >&3
  echo >&3
done
kill $HTTP_PID 2>/dev/null
wait $HTTP_PID 2>/dev/null

exec 3>&-
echo "written $RESULT"
