#!/bin/sh
# Copyright 2026 Orisema contributors
# SPDX-License-Identifier: Apache-2.0

set -eu

destination=
system_uninstall=false

usage() {
  echo "Usage: uninstall.sh (--user | --system | --bin-dir <absolute-path>)" >&2
}

fail() {
  echo "orisema uninstaller: $*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --user)
      [ -z "$destination" ] || fail "choose only one installation destination"
      destination=${XDG_BIN_HOME:-${HOME:?HOME is required}/.local/bin}/orisema
      shift
      ;;
    --system)
      [ -z "$destination" ] || fail "choose only one installation destination"
      destination=/usr/local/bin/orisema
      system_uninstall=true
      shift
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      [ -z "$destination" ] || fail "choose only one installation destination"
      destination=$2/orisema
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

[ -n "$destination" ] || fail "select --user, --system or --bin-dir"
case "$destination" in /*) ;; *) fail "installation directory must be an absolute path" ;; esac
[ -e "$destination" ] || fail "Orisema is not installed at $destination"
[ -f "$destination" ] && [ ! -L "$destination" ] || fail "destination is not a regular file"

report=$("$destination" version 2>/dev/null || true)
printf '%s\n' "$report" | grep -Fq 'Orisema Infrastructure' \
  || fail "destination does not identify itself as Orisema Infrastructure"
printf '%s\n' "$report" | grep -Eq '^Version: .+' \
  || fail "destination did not report an Orisema version"

if [ "$system_uninstall" = true ]; then
  command -v sudo >/dev/null 2>&1 || fail "required command is unavailable: sudo"
  sudo rm -- "$destination"
else
  [ "$(stat -c %u "$destination")" = "$(id -u)" ] \
    || fail "user installation must be owned by the current user"
  rm -- "$destination"
fi

echo "Removed Orisema executable at $destination"
echo "Containers, journals, manifests and application data were not changed."
