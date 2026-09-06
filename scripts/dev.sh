#!/usr/bin/env bash
set -uo pipefail

workers="${OCR_WORKERS:-3}"
threads="${OCR_THREADS:-2}"
socket_dir="${XDG_RUNTIME_DIR:-/tmp}/picseek"
pids=()

run_worker() {
	local i=$1 child="" status=0
	trap '[ -n "$child" ] && kill -TERM "$child" 2>/dev/null; wait "$child" 2>/dev/null; exit 0' TERM INT
	while true; do
		uv run --quiet picseek-ocr --socket "$socket_dir/ocr-$i.sock" --threads "$threads" &
		child=$!
		wait "$child"
		status=$?
		if [ "$status" -eq 0 ]; then
			break
		fi
		echo "ocr-$i exited with status $status, restarting in 1s" >&2
		sleep 1
	done
}

cleanup() {
	kill -TERM "${pids[@]}" 2>/dev/null || true
	wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for i in $(seq 1 "$workers"); do
	run_worker "$i" &
	pids+=($!)
done

bin/seekd "$@" &
seekd=$!
pids+=("$seekd")
wait "$seekd"
