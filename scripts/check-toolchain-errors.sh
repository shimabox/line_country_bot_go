#!/usr/bin/env bash
# Checks how scripts/install-tinygo.sh and scripts/build.sh fail, without
# changing tracked files or the tools unpacked in .tools/:
#
#   1. a TinyGo tarball whose SHA-256 does not match is rejected and nothing
#      is unpacked (a temporary copy of scripts/toolchain.env with one
#      character of the hash changed, and an empty TOOLS_DIR)
#   2. with tinygo and wasm-opt missing from PATH, build.sh stops with a
#      message that points to install-tinygo.sh
#      (a) no tools at all (empty TOOLS_DIR)
#      (b) TinyGo only (a TOOLS_DIR that links to .tools/tinygo, no binaryen/)
#   3. an unknown WASM_BUILD is rejected
#
# Run it after `bash scripts/install-tinygo.sh`. Check 1 downloads the TinyGo
# tarball, so CI does not run this. Exits 0 when every check passes and lists
# the failed checks otherwise.
set -uo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
installed_tools="${TOOLS_DIR:-$repo_root/.tools}"
unset TOOLS_DIR TOOLCHAIN_ENV WASM_BUILD

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

failures=()
pass() { echo "ok:   $1"; }
fail() {
  echo "FAIL: $1" >&2
  failures+=("$1")
}

# run NAME ENV... -- CMD...: runs CMD with ENV, saves its output to
# $tmp/NAME.log and sets rc to its exit code.
run() {
  local name=$1
  shift
  local envs=()
  while [ "$1" != "--" ]; do
    envs+=("$1")
    shift
  done
  shift
  env ${envs[@]+"${envs[@]}"} "$@" >"$tmp/$name.log" 2>&1 </dev/null
  rc=$?
  sed 's/^/      | /' "$tmp/$name.log"
}

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=LINUX_AMD64 ;;
  Darwin-arm64) platform=DARWIN_ARM64 ;;
  *)
    echo "check-toolchain-errors: unsupported platform $(uname -s) $(uname -m)" >&2
    exit 1
    ;;
esac

env_before="$(shasum -a 256 scripts/toolchain.env)"

# 1. Hash mismatch.
key="TINYGO_SHA256_${platform}"
good="$(sed -n "s/^${key}=//p" scripts/toolchain.env)"
if [ "${good: -1}" = 0 ]; then bad="${good%?}1"; else bad="${good%?}0"; fi
sed "s/^${key}=.*/${key}=${bad}/" scripts/toolchain.env >"$tmp/toolchain.env"
mkdir "$tmp/tools-empty"
if [ -z "$good" ] || ! grep -qx "${key}=${bad}" "$tmp/toolchain.env"; then
  fail "hash mismatch: could not make a copy of toolchain.env with a changed $key"
else
  echo "check: install-tinygo.sh with a wrong $key"
  run install-bad-hash TOOLS_DIR="$tmp/tools-empty" TOOLCHAIN_ENV="$tmp/toolchain.env" -- bash scripts/install-tinygo.sh
  if [ "$rc" -eq 0 ]; then
    fail "hash mismatch: install-tinygo.sh exited 0"
  elif ! grep -q "SHA-256 mismatch" "$tmp/install-bad-hash.log"; then
    fail "hash mismatch: install-tinygo.sh exited $rc without reporting a SHA-256 mismatch"
  elif [ -e "$tmp/tools-empty/tinygo" ] || [ -e "$tmp/tools-empty/binaryen" ]; then
    fail "hash mismatch: TOOLS_DIR has tinygo/ or binaryen/ after the failure"
  else
    pass "hash mismatch: exit $rc, nothing unpacked"
  fi
fi

# 2. Missing tools. PATH has only the directories of go, node and the OS's
# standard commands.
bare_path="/usr/bin:/bin:/usr/sbin:/sbin"
for cmd in go node; do
  if dir="$(dirname -- "$(command -v "$cmd")" 2>/dev/null)" && [ -n "$dir" ]; then
    bare_path="$dir:$bare_path"
  fi
done
if PATH="$bare_path" command -v tinygo >/dev/null 2>&1 || PATH="$bare_path" command -v wasm-opt >/dev/null 2>&1; then
  fail "missing tools: tinygo or wasm-opt is in the same directory as go, node or the OS commands ($bare_path)"
else
  echo "check: build.sh with no tools"
  run build-no-tools PATH="$bare_path" TOOLS_DIR="$tmp/tools-empty" -- bash scripts/build.sh
  if [ "$rc" -eq 0 ]; then
    fail "no tools: build.sh exited 0"
  elif ! grep -q "tinygo not found" "$tmp/build-no-tools.log" || ! grep -q "install-tinygo.sh" "$tmp/build-no-tools.log"; then
    fail "no tools: build.sh exited $rc without saying tinygo is missing and how to install it"
  else
    pass "no tools: exit $rc, points to install-tinygo.sh"
  fi

  if [ ! -x "$installed_tools/tinygo/bin/tinygo" ]; then
    fail "TinyGo only: $installed_tools/tinygo is not installed; run bash scripts/install-tinygo.sh first"
  else
    mkdir "$tmp/tools-tinygo-only"
    ln -s "$installed_tools/tinygo" "$tmp/tools-tinygo-only/tinygo"
    echo "check: build.sh with TinyGo but no wasm-opt"
    run build-no-wasm-opt PATH="$bare_path" TOOLS_DIR="$tmp/tools-tinygo-only" -- bash scripts/build.sh
    if [ "$rc" -eq 0 ]; then
      fail "TinyGo only: build.sh exited 0"
    elif ! grep -q "wasm-opt (Binaryen) not found" "$tmp/build-no-wasm-opt.log" || ! grep -q "install-tinygo.sh" "$tmp/build-no-wasm-opt.log"; then
      fail "TinyGo only: build.sh exited $rc without saying wasm-opt is missing and how to install it"
    else
      pass "TinyGo only: exit $rc, reports the missing wasm-opt"
    fi
  fi
fi

# 3. Unknown WASM_BUILD.
echo "check: build.sh with WASM_BUILD=foo"
run build-foo WASM_BUILD=foo -- bash scripts/build.sh
if [ "$rc" -eq 0 ]; then
  fail "WASM_BUILD=foo: build.sh exited 0"
else
  pass "WASM_BUILD=foo: exit $rc"
fi

if [ "$(shasum -a 256 scripts/toolchain.env)" != "$env_before" ]; then
  fail "scripts/toolchain.env changed"
fi

if [ "${#failures[@]}" -gt 0 ]; then
  echo "check-toolchain-errors: ${#failures[@]} check(s) failed:" >&2
  printf '  - %s\n' "${failures[@]}" >&2
  exit 1
fi
echo "check-toolchain-errors: all checks passed"
