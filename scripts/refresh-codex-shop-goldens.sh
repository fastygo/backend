#!/usr/bin/env bash
# Copy latte-shop Codex goldens from wp-fasty main into testdata.
# Usage: scripts/refresh-codex-shop-goldens.sh [checkout-dir]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/internal/conformance/testdata/wp-fasty-latte-shop"
CHECKOUT="${1:-}"
CLEANUP=""

prefer_repos=(
  "https://github.com/fastygo/wp-fasty.git"
  "https://github.com/alexy-os/wp-fasty.git"
)

clone_wp_fasty() {
  local dir="$1"
  local repo
  for repo in "${prefer_repos[@]}"; do
    if git clone --depth 1 --branch main "$repo" "$dir" >/dev/null 2>&1; then
      echo "$repo"
      return 0
    fi
  done
  echo "could not clone fastygo/wp-fasty or alexy-os/wp-fasty" >&2
  return 1
}

if [[ -z "$CHECKOUT" ]]; then
  CHECKOUT="$(mktemp -d)"
  CLEANUP="$CHECKOUT"
  REPO_URL="$(clone_wp_fasty "$CHECKOUT")"
else
  REPO_URL="$(git -C "$CHECKOUT" remote get-url origin | sed -E 's#https://[^@]+@github.com/#https://github.com/#; s#git@github.com:#https://github.com/#')"
  REPO_URL="${REPO_URL%.git}.git"
fi

COMMIT="$(git -C "$CHECKOUT" rev-parse HEAD)"
GOLDEN="$CHECKOUT/tests/fixtures/codex-draft/golden/latte-shop"
MANIFEST="$CHECKOUT/themes/latte-shop/types/codex.manifest.json"
DELIVERY="$CHECKOUT/themes/latte-shop/types/codex.delivery.json"

if [[ ! -d "$GOLDEN" || ! -f "$MANIFEST" ]]; then
  echo "latte-shop goldens or manifest missing in $CHECKOUT" >&2
  exit 1
fi

rm -rf "$DEST/golden"
mkdir -p "$DEST/golden"
cp "$GOLDEN"/*.json "$DEST/golden/"
cp "$MANIFEST" "$DEST/codex.manifest.json"

python3 - "$DELIVERY" "$DEST/codex.delivery.json" <<'PY'
import json, sys
src, dest = sys.argv[1], sys.argv[2]
delivery = {}
if src:
    try:
        with open(src, encoding="utf-8") as fh:
            delivery = json.load(fh)
    except FileNotFoundError:
        pass
if "wordpress" in delivery:
    delivery["wp"] = delivery.pop("wordpress")
with open(dest, "w", encoding="utf-8") as fh:
    json.dump(delivery, fh, indent=2, ensure_ascii=False)
    fh.write("\n")
PY

cat > "$DEST/SOURCE" <<EOF
repo=$REPO_URL
commit=$COMMIT
branch=main
golden=tests/fixtures/codex-draft/golden/latte-shop
manifest=themes/latte-shop/types/codex.manifest.json
delivery=themes/latte-shop/types/codex.delivery.json
note=fastygo/wp-fasty is tried first; alexy-os/wp-fasty is the public fallback. The delivery JSON key "wordpress" is stored as "wp". Origin.system in golden bodies is left unchanged. Shop demo PHP is WordPress-only; the HTTP test loads these portable Codex records.
EOF

if [[ -n "$CLEANUP" ]]; then
  rm -rf "$CLEANUP"
fi

echo "refreshed $DEST from $REPO_URL @$COMMIT"
