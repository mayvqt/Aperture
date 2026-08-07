#!/bin/sh
set -eu
umask 077

PUID="${PUID:-99}"
PGID="${PGID:-100}"

for ID_VALUE in "$PUID" "$PGID"; do
  case "$ID_VALUE" in
    ''|*[!0-9]*)
      echo "PUID and PGID must be numeric" >&2
      exit 1
      ;;
  esac
done

if [ "$PUID" = "0" ] || [ "$PGID" = "0" ]; then
  echo "PUID and PGID must be greater than zero" >&2
  exit 1
fi

if [ "$(id -u)" = "0" ]; then
  TARGET_GROUP="aperture"
  CURRENT_GID="$(getent group aperture | cut -d: -f3)"
  if [ "$CURRENT_GID" != "$PGID" ]; then
    EXISTING_GROUP="$(awk -F: -v gid="$PGID" '$3 == gid { print $1; exit }' /etc/group)"
    if [ -n "$EXISTING_GROUP" ]; then
      TARGET_GROUP="$EXISTING_GROUP"
      usermod -g "$TARGET_GROUP" aperture
    else
      groupmod -o -g "$PGID" aperture
    fi
  fi

  CURRENT_UID="$(id -u aperture)"
  if [ "$CURRENT_UID" != "$PUID" ]; then
    usermod -o -u "$PUID" aperture
  fi

  mkdir -p /config
  chown -R aperture:"$TARGET_GROUP" /config

  exec su-exec aperture "$@"
fi

exec "$@"
