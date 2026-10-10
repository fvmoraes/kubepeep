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
				runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tsuccess\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess'
				;;
			recover)
				reading=$((reading + 1))
				case "$reading" in
					1) runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tpending\n103\t01 · Native runtime (Windows)\tpending' ;;
					2) runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tfailure\n103\t01 · Native runtime (Windows)\tpending\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tpending' ;;
					*) runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tfailure\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess\n204\t01 · Native runtime (macOS)\tsuccess' ;;
				esac
				;;
			hardfail)
				runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tfailure\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess'
				;;
			cancel_recovered)
				reading=$((reading + 1))
				case "$reading" in
					1|2) runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tcancelled\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess' ;;
					*) runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tcancelled\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess\n205\t01 · Native runtime (macOS)\tsuccess' ;;
				esac
				;;
			missing_windows)
				runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess'
				;;
			legacy_names)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime (macos-latest)\tsuccess\n103\tnative-runtime (windows-latest)\tsuccess\n104\trestricted-kind\tsuccess'
				;;
			newer_failure)
				runs=$'101\t01 · Build and test (Linux)\tsuccess\n102\t01 · Native runtime (macOS)\tsuccess\n103\t01 · Native runtime (Windows)\tsuccess\n104\t02 · Kubernetes integration (Kind, restricted RBAC)\tsuccess\n205\t01 · Build and test (Linux)\tfailure'
				;;
			timeout)
				runs=$'101\t01 · Build and test (Linux)\tpending'
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
assert legacy_names "OUTCOME: timeout" 0
assert newer_failure "OUTCOME: aborted" 0

echo "gate-harness: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
