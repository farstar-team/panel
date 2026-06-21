#!/usr/bin/env bash
set -Eeuo pipefail

[[ "$(id -u)" -eq 0 ]] || { echo "Run as root." >&2; exit 1; }

PURGE=false
NO_CONFIRM=false
for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=true ;;
    --yes) NO_CONFIRM=true ;;
    *) echo "Unknown option: $arg" >&2; exit 2 ;;
  esac
done

if $PURGE && ! $NO_CONFIRM; then
  printf 'This permanently deletes all Farstar tunnels, secrets, logs, and settings.\n'
  read -r -p "Type DELETE to continue: " confirmation
  [[ "$confirmation" == "DELETE" ]] || { echo "Cancelled."; exit 0; }
fi

systemctl disable --now farstar.service 2>/dev/null || true
rm -f /etc/systemd/system/farstar.service
systemctl daemon-reload
rm -f /usr/local/bin/farstar

if $PURGE; then
  [[ "/etc/farstar" == /etc/farstar ]] || exit 1
  rm -rf -- /etc/farstar
  userdel farstar 2>/dev/null || true
  groupdel farstar 2>/dev/null || true
  echo "Farstar and all stored data were removed."
else
  echo "Farstar was removed. Data remains in /etc/farstar."
  echo "Reinstalling later will restore the existing configuration."
fi
