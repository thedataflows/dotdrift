#!/usr/bin/env bash
# Host-side runner for the dotdrift e2e suite. Builds one image per
# debian-family OS and runs the in-container scenario in each. Streams
# output, prints a per-container PASS/FAIL summary, and exits non-zero if
# any container fails.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

OSES="debian ubuntu"
overall=0
results=""

for os in $OSES; do
	image="dotdrift-e2e:$os"
	echo
	echo "=== building $image ==="
	if ! docker build -f "tests/e2e/Dockerfile.$os" -t "$image" .; then
		echo "FAIL: $os (image build failed)"
		results="$results $os:FAIL(build)"
		overall=1
		continue
	fi
	echo
	echo "=== running $image ==="
	if docker run --rm "$image"; then
		results="$results $os:PASS"
	else
		results="$results $os:FAIL"
		overall=1
	fi
done

echo
echo "=== e2e summary ==="
for r in $results; do
	os="${r%%:*}"
	outcome="${r##*:}"
	echo "$outcome: $os"
done

exit "$overall"
