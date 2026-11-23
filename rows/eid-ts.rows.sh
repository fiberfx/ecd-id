#!/bin/sh
# eid-ts.rows.sh: the gates of the TypeScript package ts/ (@fiberfx/eid). Run it from the root of a tree that holds
# ts vectors spec rows registry, on a machine with bun; it needs no network. Each gate prints one line, "ROW RESULT [key]
# detail", and the last line is "RESULT ...".
# Rows:
#   eid-ts-tools     bun is present (its version is printed, with node and tsc when present)
#   eid-ts-test      `bun test` in ts/ passes; every vector file reports that it checked all of its data rows
#   eid-ts-brd14     `bun test ./test/brd14.test.ts`: every row of vectors/brd14.tsv both ways, pure and through the
#                    registry layer, every refusal with its reason; order; 10000 random ids return (eid-brd14 in TypeScript)
#   eid-ts-typecheck `tsc --noEmit` on ts/tsconfig.json (strict) is clean; could-not-measure when the image carries
#                    no TypeScript compiler
#   eid-ts-mutants   control: the unmodified copy passes; each planted mutant (two alphabet characters swapped, the
#                    pad check removed, writer and count swapped in fromBrd14) is built in a scratch copy and must make `bun test` fail
# Exit: 0 no FAIL and no COULD-NOT-MEASURE; 1 any FAIL; 2 a COULD-NOT-MEASURE and no FAIL.
set -u
W=$(pwd)
L=/tmp/eid-ts
rm -rf "$L"
mkdir -p "$L"
fail=0
cnm=0
export PATH="/root/.bun/bin:/usr/local/bin:/usr/local/lib/node_modules/.bin:$PATH"
export LANG=C.UTF-8 LC_ALL=C.UTF-8 CI=1

row() { # row KEY RESULT DETAIL
	echo "ROW $2 [$1] $3"
	case "$2" in FAIL) fail=1 ;; COULD-NOT-MEASURE) cnm=1 ;; esac
}

echo "PROBE tree $W: $(ls "$W" | tr '\n' ' ')"
echo "PROBE ts: $(ls "$W/ts" 2>&1 | tr '\n' ' ')"
echo "PROBE vectors: $(ls "$W/vectors" 2>&1 | tr '\n' ' ')"

# ---- tools
if command -v bun >/dev/null 2>&1; then
	bunv=$(bun --version 2>&1)
	nodev=$(node --version 2>&1 || echo absent)
	tscbin=$(command -v tsc 2>/dev/null || true)
	echo "PROBE bun $bunv; node $nodev; tsc ${tscbin:-absent}"
	row eid-ts-tools PASS "bun $bunv"
	haveBun=1
else
	row eid-ts-tools FAIL "bun is not on PATH ($PATH)"
	haveBun=0
fi

# ---- the tests
if [ "$haveBun" -eq 1 ] && [ -d "$W/ts" ]; then
	(cd "$W/ts" && bun test) >"$L/test.log" 2>&1
	rc=$?
	grep -E '^VECTORS ' "$L/test.log" | sed 's/^/  /'
	pass=$(grep -E -o '^ *[0-9]+ pass' "$L/test.log" | grep -E -o '[0-9]+' | head -1)
	nfail=$(grep -E -o '^ *[0-9]+ fail' "$L/test.log" | grep -E -o '[0-9]+' | head -1)
	if [ $rc -ne 0 ]; then grep -E -B2 -A14 'error|fail|✗' "$L/test.log" | head -80 | cut -c1-220 | sed 's/^/  bun test: /'; fi
	files=0
	short=0
	for f in valid invalid binary_invalid lease stream stream_invalid brd14; do
		line=$(grep -E "^VECTORS $f\.tsv checked [0-9]+ of [0-9]+$" "$L/test.log" | head -1)
		if [ -z "$line" ]; then
			echo "  no VECTORS line for $f.tsv"
			short=1
			continue
		fi
		a=$(echo "$line" | awk '{print $4}')
		b=$(echo "$line" | awk '{print $6}')
		# the file's own data rows, counted here independently of the test
		d=$(grep -v '^#' "$W/vectors/$f.tsv" | grep -c .)
		[ "$a" = "$b" ] && [ "$b" = "$d" ] && [ "$a" -gt 0 ] || short=1
		files=$((files + 1))
	done
	if [ $rc -eq 0 ] && [ "${nfail:-1}" = 0 ] && [ "${pass:-0}" -gt 0 ] && [ $short -eq 0 ] && [ $files -eq 7 ]; then
		row eid-ts-test PASS "bun test: $pass pass, 0 fail; 7 vector files, every data row checked"
	else
		row eid-ts-test FAIL "rc=$rc pass=${pass:-?} fail=${nfail:-?} vector files with a full count=$files short=$short"
	fi
else
	row eid-ts-test FAIL "no bun or no ts/"
fi

# ---- the brd14 embedding: eid-brd14 in TypeScript
if [ "$haveBun" -eq 1 ] && [ -f "$W/ts/test/brd14.test.ts" ]; then
	(cd "$W/ts" && bun test ./test/brd14.test.ts) >"$L/brd14.log" 2>&1
	rc=$?
	grep -E '^(VECTORS brd14|BRD14 )' "$L/brd14.log" | sed 's/^/  /'
	if [ $rc -ne 0 ]; then grep -E -B2 -A14 'error|fail|✗' "$L/brd14.log" | head -60 | cut -c1-220 | sed 's/^/  bun test brd14: /'; fi
	line=$(grep -E '^VECTORS brd14\.tsv checked [0-9]+ of [0-9]+$' "$L/brd14.log" | head -1)
	a=$(echo "$line" | awk '{print $4}')
	d=$(grep -v '^#' "$W/vectors/brd14.tsv" 2>/dev/null | grep -c .)
	pass=$(grep -E -o '^ *[0-9]+ pass' "$L/brd14.log" | grep -E -o '[0-9]+' | head -1)
	if [ $rc -eq 0 ] && [ -n "$line" ] && [ "$a" = "$d" ] && [ "$d" -gt 0 ] && grep -q '^BRD14 ' "$L/brd14.log"; then
		row eid-ts-brd14 PASS "bun test ./test/brd14.test.ts: $pass pass; $a of $d rows of vectors/brd14.tsv both ways, pure and registry layer, every reason; order; random ids return"
	else
		row eid-ts-brd14 FAIL "rc=$rc rows checked=${a:-none} of $d"
	fi
else
	row eid-ts-brd14 FAIL "no bun or no ts/test/brd14.test.ts"
fi

# ---- the typecheck
TSC=""
if command -v tsc >/dev/null 2>&1; then
	TSC="tsc"
else
	# any typescript/lib/tsc.js the image carries: global installs, a bun cache, a node_modules of the tree
	j=$(find / -xdev \( -path /proc -o -path /sys -o -path /dev \) -prune -o -path '*typescript/lib/tsc.js' -print 2>/dev/null | head -1)
	echo "PROBE typescript search: ${j:-none}"
	if [ -n "$j" ]; then
		if command -v node >/dev/null 2>&1; then TSC="node $j"; else TSC="bun $j"; fi
	fi
fi
if [ -z "$TSC" ]; then
	row eid-ts-typecheck COULD-NOT-MEASURE "no tsc and no typescript/lib/tsc.js in the image, and the run has no network; bun test strips types and does not check them"
else
	echo "PROBE typescript: $TSC ($($TSC --version 2>&1))"
	(cd "$W/ts" && $TSC --noEmit -p tsconfig.json) >"$L/tsc.log" 2>&1
	rc=$?
	head -40 "$L/tsc.log" | cut -c1-220 | sed 's/^/  tsc: /'
	if [ $rc -eq 0 ]; then row eid-ts-typecheck PASS "$($TSC --version 2>&1): tsc --noEmit on ts/tsconfig.json (strict) is clean"; else row eid-ts-typecheck FAIL "tsc rc=$rc"; fi
fi

# ---- planted mutants
if [ "$haveBun" -eq 1 ] && [ -d "$W/ts" ]; then
	M=$L/mut
	mkdir -p "$M"
	cp -r "$W/ts" "$W/vectors" "$W/registry" "$M/"
	rm -rf "$M/ts/node_modules"
	(cd "$M/ts" && bun test) >"$L/mut-control.log" 2>&1
	crc=$?
	ok=1
	detail=""
	[ $crc -eq 0 ] || { ok=0; detail="control (unmodified copy) rc=$crc"; }
	# mutant name | file | sed expression
	for m in \
		"alphabet-swap|src/id.ts|s/'234567abcdefghij/'234567bacdefghij/" \
		"pad-check-removed|src/id.ts|s/if ((last \& 3) !== 0) throw new Refusal('pad');//" \
		"brd14-writer-count-swap|src/brd14.ts|s/Number((snow >> 12n) & 1023n), Number(snow & 4095n)/Number(snow \& 4095n), Number((snow >> 12n) \& 1023n)/"; do
		name=${m%%|*}
		rest=${m#*|}
		file=${rest%%|*}
		expr=${rest#*|}
		cp "$W/ts/$file" "$M/ts/$file"
		sed -i "$expr" "$M/ts/$file"
		if cmp -s "$W/ts/$file" "$M/ts/$file"; then
			ok=0
			detail="$detail; $name: the patch changed nothing"
			continue
		fi
		(cd "$M/ts" && bun test) >"$L/mut-$name.log" 2>&1
		rc=$?
		nf=$(grep -E -o '^ *[0-9]+ fail' "$L/mut-$name.log" | grep -E -o '[0-9]+' | head -1)
		echo "  mutant $name: bun test rc=$rc fail=${nf:-?}"
		if [ $rc -eq 0 ]; then ok=0; detail="$detail; $name: bun test still passes"; else detail="$detail; $name: bun test fails ($nf failing)"; fi
		cp "$W/ts/$file" "$M/ts/$file"
	done
	if [ $ok -eq 1 ]; then
		row eid-ts-mutants PASS "unmodified copy passes${detail}"
	else
		row eid-ts-mutants FAIL "${detail#; }"
	fi
else
	row eid-ts-mutants FAIL "no bun or no ts/"
fi

if [ $fail -ne 0 ]; then
	echo "RESULT FAIL"
	exit 1
elif [ $cnm -ne 0 ]; then
	echo "RESULT COULD-NOT-MEASURE (no FAIL; see the rows above)"
	exit 2
fi
echo "RESULT PASS"
exit 0
