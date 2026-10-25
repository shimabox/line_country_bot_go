#!/usr/bin/env bash
# Builds build/app.wasm, build/worker.mjs and build/wasm_exec.js for the Worker.
# `npm run build` and wrangler's build.command run this.
#
# Environment:
#   WASM_BUILD  tinygo (default) or go. go builds with the standard Go toolchain
#               (`npm run build:go`, `WASM_BUILD=go npx wrangler deploy`).
#   TINYGO_GC   passed to TinyGo's -gc as is (default: leaking). leaking only
#               allocates and never collects, which is safe because workers-go
#               starts a new wasm instance for every request and drops it
#               afterwards. precise is TinyGo's default for the wasm target
#               and the GC used before leaking
#               (`TINYGO_GC=precise npx wrangler deploy`). Ignored when
#               WASM_BUILD=go.
#   TOOLS_DIR   where scripts/install-tinygo.sh unpacked TinyGo and Binaryen
#               (default: .tools in the repository root). Their bin directories
#               are put at the front of PATH when they exist; otherwise tinygo
#               and wasm-opt are taken from PATH.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

build_tinygo() {
  # shellcheck source=toolchain.env
  . "$repo_root/scripts/toolchain.env"
  local tools_dir="${TOOLS_DIR:-$repo_root/.tools}"
  if [ -d "$tools_dir/binaryen/bin" ]; then PATH="$tools_dir/binaryen/bin:$PATH"; fi
  if [ -d "$tools_dir/tinygo/bin" ]; then PATH="$tools_dir/tinygo/bin:$PATH"; fi
  export PATH

  local hint="run \`bash scripts/install-tinygo.sh\` to install TinyGo ${TINYGO_VERSION} and Binaryen ${BINARYEN_VERSION} into .tools/, or set WASM_BUILD=go to build with Go"
  if ! command -v tinygo >/dev/null 2>&1; then
    echo "build: tinygo not found. $hint" >&2
    exit 1
  fi
  local version
  version="$(tinygo version | awk '{print $3}')"
  if [ "$version" != "$TINYGO_VERSION" ]; then
    echo "build: tinygo $version found at $(command -v tinygo), want $TINYGO_VERSION. $hint" >&2
    exit 1
  fi
  if ! command -v wasm-opt >/dev/null 2>&1; then
    echo "build: wasm-opt (Binaryen) not found; TinyGo needs it for wasm. $hint" >&2
    exit 1
  fi

  go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=tinygo
  tinygo build -o ./build/app.wasm -target wasm -no-debug -gc="${TINYGO_GC:-leaking}" ./cmd/worker
}

build_go() {
  go run github.com/syumai/workers-go/cmd/workers-assets-gen -mode=go
  GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o ./build/app.wasm ./cmd/worker
}

case "${WASM_BUILD:-tinygo}" in
  tinygo) build_tinygo ;;
  go) build_go ;;
  *)
    echo "build: unknown WASM_BUILD=${WASM_BUILD} (use tinygo or go)" >&2
    exit 2
    ;;
esac
