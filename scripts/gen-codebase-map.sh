#!/usr/bin/env bash
# Regenerates the package table in docs/development/codebase-map.md from the
# first sentence of each Go package's doc comment, so the table can't drift
# from the code. Fails if a package has no "// Package <name> ..." comment.
#
# Usage: scripts/gen-codebase-map.sh [--check]
#   --check  write nothing; exit 1 with a diff if the table is stale.
set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
map="docs/development/codebase-map.md"
start='<!-- generated: code-map -->'
end='<!-- /generated: code-map -->'
cd "$root"

# Prints the first sentence of the "// Package" comment directly above the
# package clause in any non-test file of directory $1, without "Package <name> ".
synopsis() {
  local f
  for f in "$1"/*.go; do
    [[ $f == *_test.go ]] && continue
    awk '
      /^\/\/ Package / { c = ""; in_doc = 1 }
      in_doc && /^\/\// { line = $0; sub(/^\/\/ ?/, "", line); c = c (c == "" ? "" : " ") line; next }
      /^package / { if (c != "") print c; exit }
      { in_doc = 0; c = "" }
    ' "$f"
  done | head -n 1 | sed -E 's/^Package [^ ]+ //; s/^(.)/\U\1/; s/([.!?])( .*)?$/\1/; s/\|/\\|/g'
}

generate() {
  local dir doc missing=0
  echo "$start"
  echo '| Package | Purpose |'
  echo '| --- | --- |'
  while IFS= read -r dir; do
    doc="$(synopsis "$dir")"
    if [[ -z $doc ]]; then
      echo "$dir: missing a \"// Package ...\" doc comment" >&2
      missing=1
    fi
    echo "| \`$dir\` | $doc |"
  done < <(find . -name '*.go' ! -name '*_test.go' ! -path './.git/*' ! -path '*/testdata/*' ! -path './vendor/*' \
             -printf '%h\n' | sed 's|^\./||' | sort -u)
  echo "$end"
  return "$missing"
}

if [[ $(grep -cxF -e "$start" -e "$end" "$map") -ne 2 ]]; then
  echo "$map: expected one '$start' ... '$end' block" >&2
  exit 1
fi

block="$(mktemp)" updated="$(mktemp)"
trap 'rm -f "$block" "$updated"' EXIT
generate > "$block"
awk -v s="$start" -v e="$end" -v f="$block" '
  $0 == s { while ((getline l < f) > 0) print l; skip = 1; next }
  $0 == e { skip = 0; next }
  !skip
' "$map" > "$updated"

if [[ ${1:-} == --check ]]; then
  if ! diff -u --label "$map" --label generated "$map" "$updated" >&2; then
    echo "$map is stale; run scripts/gen-codebase-map.sh" >&2
    exit 1
  fi
else
  cat "$updated" > "$map"
fi
