#!/usr/bin/env bash
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
bin_dir="$HOME/.local/bin"
lib_dir="$HOME/.local/lib/picseek"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
workers=3
units=(picseek-embed.service seekd.service)
for i in $(seq 1 "$workers"); do
	units=("picseek-ocr@$i.service" "${units[@]}")
done

fail() {
	echo "install: $*" >&2
	exit 1
}

[ "$(uname -s)" = Linux ] || fail "only Linux with systemd is supported for now"
for tool in systemctl go uv make; do
	command -v "$tool" >/dev/null || fail "$tool is required"
done
systemctl --user show-environment >/dev/null 2>&1 || fail "no systemd user session"

make -C "$repo" build
mkdir -p "$bin_dir" "$lib_dir" "$unit_dir"
install -m 0755 "$repo/bin/seekd" "$repo/bin/picseek" "$bin_dir/"

for service in ocr embed; do
	UV_PROJECT_ENVIRONMENT="$lib_dir/$service" uv sync --project "$repo" --package "picseek-$service" \
		--frozen --no-dev --no-editable --quiet
done
"$lib_dir/ocr/bin/picseek-ocr" --download-models --log-level WARNING

install -m 0644 "$repo"/packaging/systemd/*.service "$unit_dir/"
systemctl --user daemon-reload
systemctl --user enable "${units[@]}"
systemctl --user restart "${units[@]}"

echo "picseek is installed and running."
case ":$PATH:" in
*":$bin_dir:"*) ;;
*) echo "Add $bin_dir to your PATH to use the picseek command." ;;
esac
echo "Next: picseek index <folder>"
