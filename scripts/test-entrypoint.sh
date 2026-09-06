#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
STUB_DIR="$(mktemp -d)"
trap 'rm -rf "$STUB_DIR"' EXIT HUP INT TERM

cat > "$STUB_DIR/id" <<'EOF'
#!/bin/sh
if [ "$#" -eq 1 ] && [ "$1" = "-u" ]; then
  echo 0
else
  echo 99
fi
EOF

cat > "$STUB_DIR/getent" <<'EOF'
#!/bin/sh
echo 'aperture:x:100:'
EOF

for command in groupmod usermod mkdir chown; do
  cat > "$STUB_DIR/$command" <<'EOF'
#!/bin/sh
exit 0
EOF
done

cat > "$STUB_DIR/su-exec" <<'EOF'
#!/bin/sh
shift
exec "$@"
EOF

chmod +x "$STUB_DIR"/*

assert_rejected() {
  variable="$1"
  value="$2"
  if env PATH="$STUB_DIR:$PATH" PUID=1234 PGID=1235 "$variable=$value" \
    sh "$ROOT_DIR/docker-entrypoint.sh" true >/dev/null 2>&1; then
    echo "entrypoint unexpectedly allowed $variable=$value" >&2
    exit 1
  fi
}

for variable in PUID PGID; do
  for value in 0 00 000 -1 invalid; do
    assert_rejected "$variable" "$value"
  done
done

PATH="$STUB_DIR:$PATH" PUID=1234 PGID=1235 \
  sh "$ROOT_DIR/docker-entrypoint.sh" true
