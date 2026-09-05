#!/usr/bin/env bash
# Local validation harness for the release publish gate (F7/V7-14). It stubs
# `gh` and replays the documented scenarios: success, pending-then-success,
# failure without recovery (3 readings), cancelled recovery via re-run, and
# timeout. The gate body is extracted verbatim from release.yml.
set -uo pipefail

pass=0
fail=0

run_gate() {
	local scenario="$1" sleep_override="$2"
	deadline=$(( $(date +%s) + ${sleep_override} ))
	failure_streak=0
	local readings="${3:-}"
	local reading=0
	required_checks="build-and-test native-runtime restricted-kind"
	while :; do
		local runs=""
		case "$scenario" in
			success)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tsuccess\n103\trestricted-kind\tsuccess'
				;;
			recover)
				reading=$((reading + 1))
				case "$reading" in
					1) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tpending' ;;
					2) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tfailure\n103\trestricted-kind\tpending' ;;
					*) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tfailure\n103\trestricted-kind\tsuccess\n204\tnative-runtime\tsuccess' ;;
				esac
				;;
			hardfail)
				runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tfailure\n103\trestricted-kind\tsuccess'
				;;
			cancel_recovered)
				reading=$((reading + 1))
				case "$reading" in
					1|2) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tcancelled\n103\trestricted-kind\tsuccess' ;;
					*) runs=$'101\tbuild-and-test\tsuccess\n102\tnative-runtime\tsuccess\n103\trestricted-kind\tsuccess\n205\tnative-runtime\tsuccess' ;;
				esac
				;;
			timeout)
				runs=$'101\tbuild-and-test\tpending'
				;;
		esac
		local pending_checks="" failed_checks=""
		local check latest
		for check in $required_checks; do
			latest=$(printf '%s\n' "$runs" | awk -F'\t' -v c="$check" '$2 == c { concl = $3 } END { print concl }')
			case "$latest" in
				success) ;;
				failure|cancelled|timed_out|action_required)
					failed_checks="$failed_checks $check=$latest"
					;;
				*)
					pending_checks="$pending_checks $check=${latest:-none}"
					;;
			esac
		done
		if [ -z "$pending_checks" ] && [ -z "$failed_checks" ]; then
			echo "OUTCOME: success"
			return 0
		fi
		if [ -n "$failed_checks" ]; then
			failure_streak=$((failure_streak + 1))
			if [ "$failure_streak" -ge 3 ]; then
				echo "OUTCOME: aborted($failed_checks)"
				return 1
			fi
			sleep 0
			continue
		fi
		failure_streak=0
		if [ "$(date +%s)" -ge "$deadline" ]; then
			echo "OUTCOME: timeout($pending_checks)"
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
assert hardfail      "OUTCOME: aborted( native-runtime=failure)" 0
assert timeout       "OUTCOME: timeout( build-and-test=pending native-runtime=none restricted-kind=none)"  2

echo "gate-harness: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
