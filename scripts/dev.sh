#!/usr/bin/env bash
set -uo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
export PICSEEK_MODEL_DIR="${PICSEEK_MODEL_DIR:-$repo/.models}"
export XDG_DATA_HOME="${XDG_DATA_HOME:-$repo/.data}"

workers="${OCR_WORKERS:-3}"
threads="${OCR_THREADS:-2}"
socket_dir="${PICSEEK_SOCKET_DIR:-${XDG_RUNTIME_DIR:-/tmp}/picseek}"
pids=()

run_worker() {
	local name=$1 program=$2 child="" status=0
	shift 2
	trap '[ -n "$child" ] && kill -TERM "$child" 2>/dev/null; wait "$child" 2>/dev/null; exit 0' TERM INT
	while true; do
		uv run --quiet "$program" --socket "$socket_dir/$name.sock" "$@" &
		child=$!
		wait "$child"
		status=$?
		if [ "$status" -eq 0 ]; then
			break
		fi
		echo "$name exited with status $status, restarting in 1s" >&2
		sleep 1
	done
}

cleanup() {
	kill -TERM "${pids[@]}" 2>/dev/null || true
	wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for i in $(seq 1 "$workers"); do
	run_worker "ocr-$i" picseek-ocr --threads "$threads" &
	pids+=($!)
done
run_worker embed picseek-embed --threads "$threads" &
pids+=($!)

bin/seekd "$@" &
seekd=$!
pids+=("$seekd")
wait "$seekd"
