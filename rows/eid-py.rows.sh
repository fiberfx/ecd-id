#!/bin/sh
# eid-py.rows.sh: the gates of the Python package py/ (distribution fiberfx-eid, package eid). Run it from the root of a
# tree that holds py vectors spec rows registry, on a machine with python3 3.12 or later; it needs no network and writes
# no byte-code (PYTHONDONTWRITEBYTECODE=1). Each gate prints one line, "ROW RESULT [key] detail", and the last line is
# "RESULT ...".
# Rows:
#   eid-py-tools    python3 is present and is 3.12 or later (its version is printed); could-not-measure when absent
#   eid-py-test     `python3 -m unittest discover` in py/ passes; each of the seven vector files (valid, invalid,
#                   binary_invalid, stream, stream_invalid, lease, brd14) reports that it checked all of its data rows,
#                   and the count equals the data rows of the file in vectors/, counted here independently of the tests
#   eid-py-brd14    `python3 -m unittest tests.test_brd14` passes and reports that it checked every data row of
#                   vectors/brd14.tsv (eid-brd14 in Python)
#   eid-py-mutants  control: an unmodified copy of py/ with vectors/ beside it passes; each planted mutant (two alphabet
#                   characters swapped, the pad check removed, writer and count swapped in the brd14 embedding) is
#                   built by one sed in a fresh scratch copy and must make the tests fail; a patch that does not change
#                   exactly one line is could-not-measure
# Exit: 0 no FAIL and no COULD-NOT-MEASURE; 1 any FAIL; 2 a COULD-NOT-MEASURE and no FAIL.
set -u
W=$(pwd)
L=/tmp/eid-py
rm -rf "$L"
mkdir -p "$L"
fail=0
cnm=0
export LANG=C.UTF-8 LC_ALL=C.UTF-8 PYTHONDONTWRITEBYTECODE=1

row() { # row KEY RESULT DETAIL
	echo "ROW $2 [$1] $3"
	case "$2" in FAIL) fail=1 ;; COULD-NOT-MEASURE) cnm=1 ;; esac
}

unit() { # unit DIR LOG [MODULE]: the tests of DIR/py, all of them or one module, verbose, standard error joined
	(
		cd "$1/py" || exit 1
		if [ $# -ge 3 ]; then
			PYTHONPATH=src python3 -m unittest "$3" -v
		else
			PYTHONPATH=src python3 -m unittest discover -s tests -t . -v
		fi
	) >"$2" 2>&1
}

vec() { # vec LOG FILE: the counts A and B of the line "VECTORS FILE.tsv A/B" in LOG, in va and vb (empty if absent)
	line=$(grep -E -o "VECTORS $2\.tsv [0-9]+/[0-9]+" "$1" | head -1)
	cnt=${line##* }
	va=${cnt%/*}
	vb=${cnt#*/}
	[ -n "$line" ] || { va=""; vb=""; }
}

summary() { # summary LOG: the number of tests run, the number skipped and the number failing, in ran, skipped and nf
	ran=$(grep -E '^Ran [0-9]+ tests? in ' "$1" | head -1 | awk '{print $2}')
	last=$(grep -E '^(OK|FAILED)' "$1" | tail -1)
	skipped=$(echo "$last" | grep -E -o 'skipped=[0-9]+' | grep -E -o '[0-9]+' | head -1)
	nf=$(echo "$last" | grep -E -o '(failures|errors)=[0-9]+' | grep -E -o '[0-9]+' | awk '{s += $1} END {print s + 0}')
}

trouble() { # trouble LOG LINES PREFIX: the failing parts of a run, for the reader of the rows
	grep -E -B2 -A14 'ERROR|FAIL|Traceback' "$1" | head -"$2" | cut -c1-220 | sed "s/^/  $3/"
}

echo "PROBE tree $W:$(ls "$W" | tr '\n' ' ')"
echo "PROBE py: $(ls "$W/py" 2>&1 | tr '\n' ' ')"
echo "PROBE vectors: $(ls "$W/vectors" 2>&1 | tr '\n' ' ')"

# ---- tools
if command -v python3 >/dev/null 2>&1; then
	havePy=1
	pyv=$(python3 --version 2>&1)
	echo "PROBE python3: $(command -v python3); $pyv"
	mm=$(echo "$pyv" | sed -n 's/^Python \([0-9][0-9]*\)\.\([0-9][0-9]*\).*/\1 \2/p')
	maj=${mm% *}
	min=${mm#* }
	if [ -n "$mm" ] && { [ "$maj" -gt 3 ] || { [ "$maj" -eq 3 ] && [ "$min" -ge 12 ]; }; }; then
		row eid-py-tools PASS "$pyv"
	else
		row eid-py-tools FAIL "$pyv: Python 3.12 or later is required"
	fi
else
	havePy=0
	row eid-py-tools COULD-NOT-MEASURE "python3 is not on PATH ($PATH)"
fi

# ---- the tests
if [ "$havePy" -eq 1 ] && [ -d "$W/py" ]; then
	unit "$W" "$L/test.log"
	rc=$?
	grep -E -o 'VECTORS [a-z_0-9]+\.tsv [0-9]+/[0-9]+' "$L/test.log" | sed 's/^/  /'
	summary "$L/test.log"
	if [ $rc -ne 0 ]; then trouble "$L/test.log" 80 "unittest: "; fi
	files=0
	short=0
	for f in valid invalid binary_invalid stream stream_invalid lease brd14; do
		vec "$L/test.log" "$f"
		if [ -z "$va" ]; then
			echo "  no VECTORS line for $f.tsv"
			short=1
			continue
		fi
		# the file's own data rows, counted here independently of the tests: not a comment, not blank
		d=$(grep -v '^#' "$W/vectors/$f.tsv" 2>/dev/null | grep -c .)
		if [ "$va" != "$vb" ] || [ "$vb" != "$d" ] || [ "$va" -le 0 ]; then
			short=1
			echo "  $f.tsv: checked $va of $vb, the file has $d"
		fi
		files=$((files + 1))
	done
	if [ $rc -eq 0 ] && [ "${ran:-0}" -gt 0 ] && [ $short -eq 0 ] && [ $files -eq 7 ]; then
		row eid-py-test PASS "unittest: $ran tests OK (skipped ${skipped:-0}); 7 vector files, every data row checked"
	else
		row eid-py-test FAIL "rc=$rc ran=${ran:-?} failing=${nf:-?} vector files with a line=$files short=$short"
	fi
elif [ "$havePy" -eq 0 ]; then
	row eid-py-test COULD-NOT-MEASURE "python3 is absent"
else
	row eid-py-test FAIL "no py/"
fi

# ---- the brd14 embedding: eid-brd14 in Python
if [ "$havePy" -eq 1 ] && [ -f "$W/py/tests/test_brd14.py" ]; then
	unit "$W" "$L/brd14.log" tests.test_brd14
	rc=$?
	grep -E -o 'VECTORS brd14\.tsv [0-9]+/[0-9]+' "$L/brd14.log" | sed 's/^/  /'
	summary "$L/brd14.log"
	if [ $rc -ne 0 ]; then trouble "$L/brd14.log" 60 "unittest brd14: "; fi
	vec "$L/brd14.log" brd14
	d=$(grep -v '^#' "$W/vectors/brd14.tsv" 2>/dev/null | grep -c .)
	vok=0
	[ -n "$va" ] && [ "$va" = "$vb" ] && [ "$vb" = "$d" ] && [ "$d" -gt 0 ] && vok=1
	if [ $rc -eq 0 ] && [ "${ran:-0}" -gt 0 ] && [ $vok -eq 1 ]; then
		row eid-py-brd14 PASS "unittest tests.test_brd14: $ran tests OK; $va of $d data rows of vectors/brd14.tsv checked"
	else
		row eid-py-brd14 FAIL "rc=$rc ran=${ran:-?} rows checked=${va:-none} of ${vb:-?}, file has $d"
	fi
elif [ "$havePy" -eq 0 ]; then
	row eid-py-brd14 COULD-NOT-MEASURE "python3 is absent"
else
	row eid-py-brd14 FAIL "no py/tests/test_brd14.py"
fi

# ---- planted mutants
copy() { # copy DIR: a fresh scratch tree with py/, vectors/ and registry/ (when present) and no byte-code
	rm -rf "$1"
	mkdir -p "$1"
	cp -r "$W/py" "$W/vectors" "$1/"
	if [ -d "$W/registry" ]; then cp -r "$W/registry" "$1/"; fi
	find "$1" -name __pycache__ -type d -prune -exec rm -rf {} +
}

mutant() { # mutant NAME FILE EXPR: one sed on FILE (relative to py/) in a fresh copy; the tests must fail
	name=$1
	file=$2
	expr=$3
	if [ ! -f "$W/py/$file" ]; then
		mcnm=1
		detail="$detail; $name: no py/$file"
		return
	fi
	copy "$L/mut-$name"
	sed -i "$expr" "$L/mut-$name/py/$file"
	n=$(diff "$W/py/$file" "$L/mut-$name/py/$file" | grep -c '^[<>]')
	if [ "$n" -ne 2 ]; then
		mcnm=1
		detail="$detail; $name: the patch changed $((n / 2)) lines, not exactly 1"
		return
	fi
	unit "$L/mut-$name" "$L/mut-$name.log"
	rc=$?
	summary "$L/mut-$name.log"
	echo "  mutant $name: unittest rc=$rc failing=${nf:-?}"
	if [ $rc -eq 0 ]; then
		ok=0
		detail="$detail; $name: the tests still pass"
	else
		detail="$detail; $name: the tests fail ($nf failing)"
	fi
}

# the sed expressions of the mutants; each matches one line of the package, once
MUT_ALPHABET='s/^ALPHABET = "234567abcdefghijklmnopqrstuvwxyz"$/ALPHABET = "324567abcdefghijklmnopqrstuvwxyz"/'
MUT_PAD='s/^\([[:space:]]*\)if \(.*\):  # pad check$/\1if False and (\2):  # pad check/'
MUT_BRD14='s/writer=node, count=seq)  # brd14 parts$/writer=seq, count=node)  # brd14 parts/'

if [ "$havePy" -eq 1 ] && [ -d "$W/py" ]; then
	ok=1
	mcnm=0
	detail=""
	copy "$L/mut-control"
	unit "$L/mut-control" "$L/mut-control.log"
	crc=$?
	[ $crc -eq 0 ] || { ok=0; detail="control (unmodified copy) rc=$crc"; }
	mutant alphabet-swap src/eid/id.py "$MUT_ALPHABET"
	mutant pad-check-removed src/eid/id.py "$MUT_PAD"
	mutant brd14-writer-count-swap src/eid/brd14.py "$MUT_BRD14"
	if [ $ok -eq 0 ]; then
		row eid-py-mutants FAIL "${detail#; }"
	elif [ $mcnm -eq 1 ]; then
		row eid-py-mutants COULD-NOT-MEASURE "${detail#; }"
	else
		row eid-py-mutants PASS "unmodified copy passes${detail}"
	fi
elif [ "$havePy" -eq 0 ]; then
	row eid-py-mutants COULD-NOT-MEASURE "python3 is absent"
else
	row eid-py-mutants FAIL "no py/"
fi

PROBE_PYC=$(find "$W/py" -name __pycache__ -type d 2>/dev/null | wc -l | tr -d " ")
echo "PROBE byte-code directories left in py/: $PROBE_PYC"

if [ $fail -ne 0 ]; then
	echo "RESULT FAIL"
	exit 1
elif [ $cnm -ne 0 ]; then
	echo "RESULT COULD-NOT-MEASURE (no FAIL; see the rows above)"
	exit 2
fi
echo "RESULT PASS"
exit 0
