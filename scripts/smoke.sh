#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 /path/to/kubePeep" >&2
	exit 2
fi

case "$1" in
	/*) smoke_binary=$1 ;;
	*) smoke_binary=$(pwd)/$1 ;;
esac
if [ ! -x "$smoke_binary" ]; then
	echo "smoke: binary is not executable" >&2
	exit 2
fi

smoke_root=$(mktemp -d)
smoke_home=$smoke_root/home
smoke_stdout=$smoke_root/stdout.log
smoke_stderr=$smoke_root/stderr.log
mkdir -p "$smoke_home"
smoke_pid=

cleanup() {
	if [ -n "$smoke_pid" ] && kill -0 "$smoke_pid" 2>/dev/null; then
		kill "$smoke_pid" 2>/dev/null || true
		wait "$smoke_pid" 2>/dev/null || true
	fi
	rm -rf -- "$smoke_root"
}
trap cleanup EXIT HUP INT TERM

env -i HOME="$smoke_home" PATH=/nonexistent "$smoke_binary" start --no-browser >"$smoke_stdout" 2>"$smoke_stderr" &
smoke_pid=$!

smoke_status=
smoke_attempt=0
while [ "$smoke_attempt" -lt 200 ]; do
	if ! kill -0 "$smoke_pid" 2>/dev/null; then
		echo "smoke: process exited before readiness" >&2
		sed -n '1,20p' "$smoke_stderr" >&2
		exit 1
	fi
	if smoke_status=$(env -i HOME="$smoke_home" PATH=/nonexistent "$smoke_binary" status 2>/dev/null); then
		break
	fi
	smoke_attempt=$((smoke_attempt + 1))
	sleep 0.05
done
if [ -z "$smoke_status" ]; then
	echo "smoke: instance did not publish readiness" >&2
	exit 1
fi

smoke_port=$(printf '%s\n' "$smoke_status" | sed -n 's/.*port=\([0-9][0-9]*\).*/\1/p')
case "$smoke_port" in
	''|*[!0-9]*) echo "smoke: status did not contain a valid port" >&2; exit 1 ;;
esac

curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:$smoke_port/health" >/dev/null
curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:$smoke_port/api/v1/status" >/dev/null
curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:$smoke_port/api/v1/session" >/dev/null

env -i HOME="$smoke_home" PATH=/nonexistent "$smoke_binary" stop >/dev/null
wait "$smoke_pid"
smoke_pid=

smoke_data=$smoke_home/.kubePeep
for smoke_path in \
	"$smoke_data/config.yaml" \
	"$smoke_data/kubePeep.db" \
	"$smoke_data/logs/kubePeep.log" \
	"$smoke_data/runtime/kubePeep.lock" \
	"$smoke_data/cache"
do
	if [ ! -e "$smoke_path" ]; then
		echo "smoke: expected artifact is missing" >&2
		exit 1
	fi
done
if [ -e "$smoke_data/runtime/instance.json" ]; then
	echo "smoke: instance state survived clean shutdown" >&2
	exit 1
fi

echo "smoke: embedded binary lifecycle passed without a Node.js PATH"
