#!/bin/sh
# Read GitHub check-run TSV (id, name, conclusion). Exit 0 only after every
# required check succeeds; 1 means failed and 2 means missing/pending.
set -eu

awk -F '\t' '
BEGIN {
	required[1] = "build-and-test"
	required[2] = "restricted-kind"
	required[3] = "native-runtime (macos-latest)"
	required[4] = "native-runtime (windows-latest)"
}
$1 ~ /^[0-9]+$/ && $1 + 0 > latest_id[$2] {
	latest_id[$2] = $1 + 0
	conclusion[$2] = $3
}
END {
	for (i = 1; i <= 4; i++) {
		check = required[i]
		result = conclusion[check]
		if (result == "success") continue
		if (result ~ /^(failure|cancelled|timed_out|action_required)$/) {
			failed = failed " " check "=" result
		} else {
			pending = pending " " check "=" (result == "" ? "none" : result)
		}
	}
	if (failed != "") {
		print "failed:" failed " pending:" pending
		exit 1
	}
	if (pending != "") {
		print "pending:" pending
		exit 2
	}
	print "success"
}'
