#!/bin/sh
# eid-casefold.sh VECTORS_DIR: the measurement of spec/id.md:54 that a Linux container cannot make (eid.rows.sh reports
# eid-casefold as could-not-measure): on a case-folding file system (APFS or NTFS by default), every canonical eid text
# in the vectors names its own directory, so no two distinct ids meet in one file name.
# Run it on such a host, with the vectors of the revision under test, for example from the kanbox:
#   ssh m5 'sh -s /Users/jonny/dev/ecd/ecd-id/vectors' < rows/eid-casefold.sh
# It compiles nothing and runs no test suite: it makes one directory per id in a fresh temporary directory, counts them,
# and removes the directory. Output: one line "ROW RESULT [key] detail", then "RESULT ...".
#   eid-casefold          every distinct id text from the vectors gets its own directory: N ids give N entries
#   eid-casefold-control  the file system folds case: an id's upper-case twin opens the same directory, so the entry
#                         count stays N; without folding this row is COULD-NOT-MEASURE (the property is then untested)
# Exit: 0 PASS; 1 FAIL; 2 could not measure (no vectors, no folding, or no temporary directory).
set -u
V=${1:?usage: eid-casefold.sh VECTORS_DIR}
[ -d "$V" ] || { echo "ROW COULD-NOT-MEASURE [eid-casefold] no vectors at $V"; echo "RESULT could-not-measure"; exit 2; }
T=$(mktemp -d "${TMPDIR:-/tmp}/eid-casefold.XXXXXX") || { echo "RESULT could-not-measure: no temporary directory"; exit 2; }
trap 'rm -rf "$T"' EXIT HUP INT TERM
fs=$( (diskutil info "$(df "$T" | awk 'NR == 2 {print $1}')" 2>/dev/null || true) | awk -F: '/File System Personality/ {gsub(/^ +/, "", $2); print $2}')
echo "PROBE host $(uname -sm) fs ${fs:-unknown} dir $T"

# Every 28-character canonical id text in the vectors: lower-case alphabet 2-7a-z with underscores at 4 and 8.
grep -h -o -E '\b[2-7a-z]{3}_[a-z2-7]{3}_[2-7a-z]{20}\b' "$V"/*.tsv | sort -u > "$T/ids"
n=$(wc -l < "$T/ids" | tr -d ' ')
[ "$n" -gt 0 ] || { echo "ROW COULD-NOT-MEASURE [eid-casefold] no id text found in $V"; echo "RESULT could-not-measure"; exit 2; }
mkdir "$T/d"
while IFS= read -r id; do mkdir "$T/d/$id" 2>/dev/null; done < "$T/ids"
got=$(ls "$T/d" | wc -l | tr -d ' ')
fail=0
if [ "$got" -eq "$n" ]; then
	echo "ROW PASS [eid-casefold] $n distinct id texts from the vectors give $got directories on ${fs:-this file system}"
else
	echo "ROW FAIL [eid-casefold] $n distinct id texts give $got directories"
	fail=1
fi

# The control: the upper-case twin of the first id that holds a letter (an all-digit id has no twin). On a folding
# file system mkdir fails (the entry exists) and the count stays n; the twin's spelling resolves to the same directory.
first=$(grep -m 1 "[a-z]" "$T/ids")
twin=$(printf '%s' "$first" | tr 'a-z' 'A-Z')
mkdir "$T/d/$twin" 2>/dev/null
after=$(ls "$T/d" | wc -l | tr -d ' ')
if [ "$after" -eq "$n" ] && [ -d "$T/d/$twin" ]; then
	echo "ROW PASS [eid-casefold-control] the file system folds case: $twin opens the directory of $first (entries stay $after)"
	res=PASS
else
	echo "ROW COULD-NOT-MEASURE [eid-casefold-control] the file system does not fold case ($after entries after the twin), so eid-casefold measured nothing"
	res=could-not-measure
	[ $fail -eq 0 ] && { echo "RESULT could-not-measure"; exit 2; }
fi
[ $fail -eq 0 ] && { echo "RESULT $res"; exit 0; }
echo "RESULT FAIL"
exit 1
