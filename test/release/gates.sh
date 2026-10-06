#!/usr/bin/env bash
# Local validation harness for the release publish gate (F7/V7-14). It stubs
# `gh` and replays the documented scenarios: success, pending-then-success,
# failure without recovery (3 readings), cancelled recovery via re-run, and
# timeout. Classification is shared with the publish job in release.yml.
set -uo pipefail

repository=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
classifier=$repository/scripts/release/check-status.sh

pass=0
fail=0

run_gate() {
	local scenario="$1" sleep_override="$2"
	deadline=$(( $(date +%s) + ${sleep_override} ))
	failure_streak=0
	local reading=0
	while :; do
		local runs=""
		case "$scenario" in
			success)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tsuccess\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess'
				;;
			recover)
				reading=$((reading + 1))
				case "$reading" in
					1) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tpending\n103\tnative-runtime (windows-latest)\tpending' ;;
					2) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tfailure\n103\tnative-runtime (windows-latest)\tpending\n104\trestricted-kind\tpending' ;;
					*) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tfailure\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess\n204\tnative-runtime (macos-latest)\tsuccess' ;;
				esac
				;;
			hardfail)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tfailure\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess'
				;;
			cancel_recovered)
				reading=$((reading + 1))
				case "$reading" in
					1|2) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tcancelled\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess' ;;
					*) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tcancelled\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess\n205\tnative-runtime (macos-latest)\tsuccess' ;;
				esac
				;;
			missing_windows)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tsuccess\n104\trestricted-kind\tsuccess'
				;;
			timeout)
				runs=$'101\tbuild-and-test\tpending'
				;;
		esac
		local state status
		if state=$(printf '%s\n' "$runs" | "$classifier"); then
			echo "OUTCOME: success"
			return 0
		else
			status=$?
		fi
		if [ "$status" -eq 1 ]; then
			failure_streak=$((failure_streak + 1))
			if [ "$failure_streak" -ge 3 ]; then
				echo "OUTCOME: aborted"
				return 1
			fi
			continue
		fi
		if [ "$status" -ne 2 ]; then return "$status"; fi
		failure_streak=0
		if [ "$(date +%s)" -ge "$deadline" ]; then
			echo "OUTCOME: timeout"
			return 2
		fi
		sleep 0
	done
}

assert() {
	local scenario="$1" expected="$2" timeout="$3"
	local actual
	actual=$(run_gate "$scenario" "$timeout")
	if [ "$actual" = "$expected" ]; then
		pass=$((pass + 1))
		echo "PASS  $scenario → $actual"
	else
		fail=$((fail + 1))
		echo "FAIL  $scenario → got '$actual', want '$expected'"
	fi
}

# The real gate sleeps 45s/60s; the harness replaces them with no-ops and the
# timeout scenarios use a zero-second deadline.
assert success       "OUTCOME: success"              0
assert recover       "OUTCOME: success"              2
assert cancel_recovered "OUTCOME: success"           0
assert hardfail      "OUTCOME: aborted" 0
assert timeout       "OUTCOME: timeout"  0

assert missing_windows "OUTCOME: timeout" 0

echo "gate-harness: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
