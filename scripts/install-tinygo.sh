#!/usr/bin/env bash
# Downloads TinyGo and Binaryen (wasm-opt) from their official GitHub
# releases, checks the SHA-256 in scripts/toolchain.env and unpacks them to
# $TOOLS_DIR/tinygo and $TOOLS_DIR/binaryen. scripts/build.sh puts their bin
# directories at the front of PATH.
#
# Usage:
#   bash scripts/install-tinygo.sh
#
# Environment:
#   TOOLS_DIR      where to unpack (default: .tools in the repository root)
#   TOOLCHAIN_ENV  versions and hashes to use (default: scripts/toolchain.env)
#
# Supported: linux-amd64, darwin-arm64. A tool already unpacked with the same
# version and hash is not downloaded again, so the script can be run any number
# of times. Both tarballs are verified before anything is unpacked: on a hash
# mismatch nothing is written to $TOOLS_DIR.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
tools_dir="${TOOLS_DIR:-$repo_root/.tools}"
toolchain_env="${TOOLCHAIN_ENV:-$repo_root/scripts/toolchain.env}"
marker=".toolchain-version"

if [ ! -f "$toolchain_env" ]; then
  echo "install-tinygo: $toolchain_env not found" >&2
  exit 1
fi
# shellcheck source=toolchain.env
. "$toolchain_env"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)
    platform=LINUX_AMD64
    tinygo_file="tinygo${TINYGO_VERSION}.linux-amd64.tar.gz"
    binaryen_file="binaryen-${BINARYEN_VERSION}-x86_64-linux.tar.gz"
    ;;
  Darwin-arm64)
    platform=DARWIN_ARM64
    tinygo_file="tinygo${TINYGO_VERSION}.darwin-arm64.tar.gz"
    binaryen_file="binaryen-${BINARYEN_VERSION}-arm64-macos.tar.gz"
    ;;
  *)
    {
      echo "install-tinygo: unsupported platform $(uname -s) $(uname -m)"
      echo "supported platforms:"
      echo "  linux-amd64   (Linux x86_64)"
      echo "  darwin-arm64  (macOS Apple silicon)"
    } >&2
    exit 1
    ;;
esac

tinygo_sha_var="TINYGO_SHA256_${platform}"
binaryen_sha_var="BINARYEN_SHA256_${platform}"
tinygo_sha="${!tinygo_sha_var:-}"
binaryen_sha="${!binaryen_sha_var:-}"
if [ -z "${TINYGO_VERSION:-}" ] || [ -z "${BINARYEN_VERSION:-}" ] || [ -z "$tinygo_sha" ] || [ -z "$binaryen_sha" ]; then
  echo "install-tinygo: $toolchain_env must set TINYGO_VERSION, BINARYEN_VERSION, $tinygo_sha_var and $binaryen_sha_var" >&2
  exit 1
fi

tinygo_url="https://github.com/tinygo-org/tinygo/releases/download/v${TINYGO_VERSION}/${tinygo_file}"
binaryen_url="https://github.com/WebAssembly/binaryen/releases/download/${BINARYEN_VERSION}/${binaryen_file}"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# installed NAME VERSION SHA: whether $tools_dir/NAME already holds that build.
installed() {
  [ -f "$tools_dir/$1/$marker" ] && [ "$(cat "$tools_dir/$1/$marker")" = "$2 $3" ]
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Download and verify first; unpack only after every needed tarball matches.
pending=()
fetch() {
  local name=$1 version=$2 sha=$3 url=$4 file=$5
  if installed "$name" "$version" "$sha"; then
    echo "install-tinygo: $name $version already in $tools_dir/$name"
    return
  fi
  echo "install-tinygo: downloading $url"
  curl -fsSL --retry 3 -o "$tmp/$file" "$url"
  local got
  got="$(sha256_of "$tmp/$file")"
  if [ "$got" != "$sha" ]; then
    {
      echo "install-tinygo: SHA-256 mismatch for $file"
      echo "  expected $sha"
      echo "  got      $got"
    } >&2
    exit 1
  fi
  echo "install-tinygo: $file SHA-256 ok"
  pending+=("$name|$version|$sha|$file")
}

fetch tinygo "$TINYGO_VERSION" "$tinygo_sha" "$tinygo_url" "$tinygo_file"
fetch binaryen "$BINARYEN_VERSION" "$binaryen_sha" "$binaryen_url" "$binaryen_file"

for entry in ${pending[@]+"${pending[@]}"}; do
  IFS='|' read -r name version sha file <<<"$entry"
  mkdir -p "$tools_dir"
  # Unpack next to the destination and move it into place, so an interrupted
  # run never leaves a half-unpacked tool that looks installed.
  staging="$(mktemp -d "$tools_dir/.$name.XXXXXX")"
  tar -xzf "$tmp/$file" -C "$staging" --strip-components=1
  printf '%s %s\n' "$version" "$sha" > "$staging/$marker"
  rm -rf "$tools_dir/$name"
  mv "$staging" "$tools_dir/$name"
  echo "install-tinygo: unpacked $name $version to $tools_dir/$name"
done

"$tools_dir/tinygo/bin/tinygo" version
"$tools_dir/binaryen/bin/wasm-opt" --version
