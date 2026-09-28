#!/usr/bin/env bash
# Docs check, run by CI. Enforces the documentation rules a script can check,
# so review only has to judge the rest:
#   - the generated code map is current and every Go package has a doc comment
#   - relative Markdown links resolve, including #section anchors
#   - every page under docs/ is linked from another page
#   - every Paths pattern in the documentation impact map matches a tracked
#     file, and every row names at least one page
# Usage: scripts/check-docs.sh   (prints only failures)
set -euo pipefail
set -f # link targets and table patterns must never expand against the disk

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
conventions="docs/development/documentation-conventions.md"
cd "$root"
failed=0
fail() { echo "$*" >&2; failed=1; }

scripts/gen-codebase-map.sh --check || failed=1

# Resolves link $2 relative to directory $1 into a normalized repo path.
resolve() {
  local part out=() path="$1/$2"
  local IFS=/
  for part in $path; do
    case $part in
      ''|.) ;;
      ..) unset 'out[${#out[@]}-1]' ;;
      *) out+=("$part") ;;
    esac
  done
  echo "${out[*]}"
}

# GitHub-style heading anchors of Markdown file $1, one per line.
anchors() {
  awk '/^```/ { fence = !fence } !fence && /^#+ / { sub(/^#+ +/, ""); print }' "$1" |
    tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9 _-]//g; s/ /-/g'
}

mapfile -t pages < <(git ls-files --cached --others --exclude-standard -- '*.md' | sort -u)
declare -A linked=()

for page in "${pages[@]}"; do
  [[ -f $page ]] || continue
  dir="$(dirname "$page")"
  # Links outside fenced code blocks and inline code.
  while IFS= read -r link; do
    case $link in http://*|https://*|mailto:*) continue ;; esac
    target="${link%%#*}" anchor=""
    [[ $link == *#* ]] && anchor="${link#*#}"
    if [[ -z $target ]]; then
      target="$page"
    else
      target="$(resolve "$dir" "$target")"
    fi
    if [[ ! -e $target ]]; then
      fail "$page: broken link ($link)"
      continue
    fi
    [[ $target != "$page" ]] && linked[$target]=1
    if [[ -n $anchor && $target == *.md ]] && ! anchors "$target" | grep -qxF -- "$anchor"; then
      fail "$page: no section '#$anchor' in $target"
    fi
  done < <(awk '/^```/ { fence = !fence; next } !fence { gsub(/`[^`]*`/, ""); print }' "$page" |
             grep -o '\]([^) ]*)' | sed 's/^](//; s/)$//')
done

for page in "${pages[@]}"; do
  [[ $page == docs/* && -f $page && -z ${linked[$page]:-} ]] && fail "$page: no other page links to it"
done

rows=0
while IFS=$'\t' read -r change patterns targets; do
  rows=$((rows + 1))
  # shellcheck disable=SC2016 # backticks are literal
  mapfile -t list < <(grep -o '`[^`]*`' <<<"$patterns" | tr -d '`')
  [[ ${#list[@]} -gt 0 ]] || fail "$conventions: impact-map row '$change' has no Paths"
  grep -q '\](' <<<"$targets" || fail "$conventions: impact-map row '$change' links no page"
  for pattern in "${list[@]}"; do
    matched=0
    while IFS= read -r file; do
      # shellcheck disable=SC2254 # unquoted so the pattern matches
      case $file in $pattern) matched=1; break ;; esac
    done < <(git ls-files)
    [[ $matched -eq 1 ]] || fail "$conventions: impact-map pattern '$pattern' ($change) matches no tracked file"
  done
done < <(awk -F'|' '
  /^## / { in_map = ($0 == "## Documentation impact map"); next }
  in_map && /^\|/ && ++n > 2 { gsub(/^ +| +$/, "", $2); print $2 "\t" $3 "\t" $4 }
' "$conventions")
[[ $rows -gt 0 ]] || fail "$conventions: documentation impact map not found"

if [[ $failed -ne 0 ]]; then
  echo "Docs check failed." >&2
  exit 1
fi
