#!/usr/bin/env bash
set -euo pipefail

bin_dir="$HOME/.local/bin"
lib_dir="$HOME/.local/lib/picseek"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
purge=false

case "${1:-}" in
"") ;;
--purge) purge=true ;;
*)
	echo "usage: $0 [--purge]" >&2
	exit 2
	;;
esac

mapfile -t units < <(systemctl --user list-unit-files --no-legend 'seekd.service' 'picseek-*' | awk '{print $1}')
mapfile -t running < <(systemctl --user list-units --all --no-legend --plain 'picseek-ocr@*' | awk '{print $1}')
if [ "${#units[@]}" -gt 0 ] || [ "${#running[@]}" -gt 0 ]; then
	systemctl --user disable --now "${units[@]}" "${running[@]}" 2>/dev/null || true
fi
rm -f "$unit_dir/seekd.service" "$unit_dir/picseek-embed.service" "$unit_dir/picseek-ocr@.service"
systemctl --user daemon-reload
systemctl --user reset-failed 2>/dev/null || true

rm -f "$bin_dir/seekd" "$bin_dir/picseek"
rm -rf "$lib_dir"

if "$purge"; then
	rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/picseek" \
		"${XDG_CONFIG_HOME:-$HOME/.config}/picseek" \
		"${XDG_CACHE_HOME:-$HOME/.cache}/picseek"
	echo "picseek is uninstalled, and its index, config and models are removed."
else
	echo "picseek is uninstalled. The index, config and models are kept; run with --purge to remove them."
fi
