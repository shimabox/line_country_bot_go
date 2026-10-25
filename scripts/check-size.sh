#!/usr/bin/env bash
# Builds the Worker with `wrangler deploy --dry-run` and fails when the gzip
# size exceeds the limit. The free plan allows 3MB after compression; the
# default limit of 2560 KiB (2.5MiB) keeps a margin. The wrangler output is
# shown even when the dry-run (and so the build) fails, and the script then
# exits with wrangler's exit code.
set -euo pipefail

limit_kib="${1:-2560}"

rc=0
out="$(npx wrangler deploy --dry-run --outdir=dist 2>&1)" || rc=$?
echo "$out"
if [ "$rc" -ne 0 ]; then
  exit "$rc"
fi

gzip_kib="$(echo "$out" | sed -nE 's/.*gzip: ([0-9.]+) KiB.*/\1/p' | tail -n 1)"
if [ -z "$gzip_kib" ]; then
  echo "check-size: could not read the gzip size" >&2
  exit 1
fi

echo "check-size: gzip ${gzip_kib} KiB (limit ${limit_kib} KiB)"
if ! awk -v s="$gzip_kib" -v l="$limit_kib" 'BEGIN { exit !(s <= l) }'; then
  echo "check-size: over the limit" >&2
  exit 1
fi
