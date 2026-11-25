#!/bin/sh
# eid.rows.sh: the gates of the Ecd ID (eid), version 1. Run it from the root of a tree that holds README.md spec
# registry model vectors rs go, on a machine with go and cargo (and, for eid-valkey-live, a valkey-server); it needs no
# network. Each gate prints one line, "ROW RESULT [key] detail", and the last line is "RESULT ...".
# Rows (one ROW line per gate; a gate passes when every language present passes it):
#   eid-oracle      go vet of model/ is clean and the committed vectors equal the Go model's output (go run . --check)
#   eid-oracle-control  the same check on a scratch copy of vectors/ with one byte of one data line changed fails
#   eid-vectors     G1   Rust and Go agree with every vector both ways; every invalid input refused with its reason
#   eid-order       G2   1e6 random ids sort the same as text and bytes; the a-z2-7 control breaks order
#   eid-burst       G3   8 leases, 1e7 ids, unique and rising; borrowing; two kinds; control: one lease, two minters
#   eid-stream      G4   two systems on one writer and ms: distinct stream ids in order, inverse; control: 48-bit tail
#   eid-valkey-live G4   the same against valkey-server, when the image has one
#   eid-restart     G5   1000 restarts, some inside one second; control: the wait removed
#   eid-clock-back  G6   restore with a correct clock; clock behind refused; restore plus clock back counted
#   eid-fenced-clone G7  other home, second activation, pid guard; controls: home bypassed, guard blind
#   eid-mxdb        G9   owner, store and database encodings; all-zero refused; no time-0 mint
#   eid-door        G10  detached ids bound, never stored as own; control: no door
#   eid-one-case    G11  the canonical text is one case (the vectors)
#   eid-casefold    G11  directories on a case-folding file system: COULD-NOT-MEASURE in a Linux container; measured
#                        on such a host by rows/eid-casefold.sh
#   eid-registry         the registry files load; faults refused
#   eid-registry-retired every kind row of a retired system is retired (the 39 of brd); ecd sto is present and live
#   eid-brd14       G14  brd14 ids embed into brd ids: Rust and Go agree with every row of vectors/brd14.tsv both ways, pure
#                        and through the registry layer, every refusal with its reason; order; 1e5 random ids return
#   eid-brd14-control    go: the unmodified scratch copy passes G14; with writer and count swapped in FromBrd14, G14 fails
#   eid-registry-retired-control  a scratch copy of registry/ passes eid-registry-retired unmodified and fails it, in Go
#                        and Rust, with brd crs turned back to live
#   eid-encodings        go: MarshalText/UnmarshalText, Value/Scan and NullID with JSON round-trip every line of
#                        vectors/valid.tsv; every line of vectors/invalid.tsv is refused as Parse refuses it; the zero
#                        id is refused (ErrZero)
#   eid-encodings-control  go: the unmodified scratch copy passes the encodings; with the zero refusal of MarshalText
#                        removed by one sed, they fail
#   eid-registry-roles-control  go: the unmodified scratch copy passes the role tests (TestRegistryRoles and
#                        TestRegistryKindsByRole); with the role test of KindsByRole removed by one sed, they fail
#   eid-detached-draw    one 16-bit draw, r >= 61440 rejected, code 1024 + r mod 5120; Rust and Go agree; 64 rejections
#                        refused
#   eid-store       G12  go: two processes x 1000 grants on one FileStore give 2000 distinct writers; control: no lock
#   eid-cli         G13  go: the command eid against vectors/cli.tsv, in process; stores, refusals, detached
#   eid-lease            go: TakeLease on a FileStore: distinct writers per call, the activation released, refusals; control: the
#                        start-wait removed repeats a writer (the command's lease tests are in eid-cli)
#   eid-bin              go: eid built for linux-arm64, darwin-arm64 and linux-amd64 (CGO off), each twice with one
#                        sha256; the vectors run against the linux-amd64 build; the three binaries are printed as
#                        gzip+base64 between BIN-BEGIN and BIN-END lines
# Exit: 0 no FAIL and no unexpected COULD-NOT-MEASURE; 1 any FAIL; 2 an unexpected COULD-NOT-MEASURE.
set -u
W=$(pwd)
L=/tmp/eid
mkdir -p "$L"
fail=0
cnm=0
export PATH="/usr/local/go/bin:/usr/lib/go/bin:/root/.cargo/bin:/usr/local/cargo/bin:$PATH"
export CARGO_TARGET_DIR=/tmp/eid-target CARGO_NET_OFFLINE=true
export GOFLAGS=-mod=mod GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOCACHE=/tmp/gocache GOPATH=/tmp/gopath
export LANG=C.UTF-8 LC_ALL=C.UTF-8

row() { # row KEY RESULT DETAIL
	echo "ROW $2 [$1] $3"
	case "$2" in FAIL) fail=1 ;; esac
}

echo "PROBE tree $W: $(ls "$W" | tr '\n' ' ')"
echo "PROBE $(cargo --version 2>&1); $(rustc --version 2>&1); $(go version 2>&1)"
echo "PROBE cpus $(nproc 2>/dev/null) mem $(awk '/MemTotal/{print $2" kB"}' /proc/meminfo 2>/dev/null)"

# ---- the oracle
(cd "$W/model" && go vet ./...) >"$L/oracle-vet.log" 2>&1
vet=$?
sed 's/^/  oracle vet: /' "$L/oracle-vet.log"
gofmt -l "$W/model" >"$L/oracle-fmt.log" 2>&1
[ -s "$L/oracle-fmt.log" ] && sed 's/^/  gofmt would change: /' "$L/oracle-fmt.log"
(cd "$W/model" && go run . --check) >"$L/oracle.log" 2>&1
rc=$?
sed 's/^/  oracle: /' "$L/oracle.log"
if [ $rc -ne 0 ]; then
	# What the model would write: a file the tree lacks is printed whole between REGEN-BEGIN and REGEN-END, a file
	# that differs as a diff.
	R=$(mktemp -d /tmp/eid-regen.XXXXXX)
	cp -r "$W/model" "$W/vectors" "$W/registry" "$R/"
	(cd "$R/model" && go run .) >"$L/regen.log" 2>&1
	sed 's/^/  regen: /' "$L/regen.log"
	for f in "$R"/vectors/*.tsv; do
		b=$(basename "$f")
		if [ ! -f "$W/vectors/$b" ]; then
			echo "REGEN-BEGIN $b"
			cat "$f"
			echo "REGEN-END $b"
		elif ! cmp -s "$f" "$W/vectors/$b"; then
			diff -u "$W/vectors/$b" "$f" | head -40 | sed 's/^/  regen diff: /'
		fi
	done
	rm -rf "$R"
fi
if [ $rc -eq 0 ] && [ $vet -eq 0 ]; then
	row eid-oracle PASS "go vet of model/ is clean; vectors equal the output of the Go model in model/"
else
	row eid-oracle FAIL "go vet rc=$vet, check rc=$rc"
fi

# The violating case: one byte of one data line of valid.tsv is changed in a scratch copy; --check must fail on that
# file alone.
S=$(mktemp -d /tmp/eid-control.XXXXXX)
cp -r "$W/model" "$W/vectors" "$W/registry" "$S/"
n=$(grep -n -v '^#' "$S/vectors/valid.tsv" | head -1 | cut -d: -f1)
sed -i "${n}s/^./Z/" "$S/vectors/valid.tsv"
bytes=$(cmp -l "$W/vectors/valid.tsv" "$S/vectors/valid.tsv" | wc -l)
(cd "$S/model" && go run . --check) >"$L/oracle-control.log" 2>&1
crc=$?
sed 's/^/  oracle control: /' "$L/oracle-control.log"
if [ "$bytes" -eq 1 ] && [ $crc -eq 1 ] && grep -qx 'DIFF valid.tsv' "$L/oracle-control.log" &&
	[ "$(grep -c '^DIFF' "$L/oracle-control.log")" -eq 1 ]; then
	row eid-oracle-control PASS "one byte of valid.tsv changed in a scratch copy: --check exits 1 and names valid.tsv only"
else
	row eid-oracle-control FAIL "bytes changed=$bytes, check rc=$crc"
fi
rm -rf "$S"

# ---- Rust
rust=absent
if command -v cargo >/dev/null 2>&1 && [ -f "$W/rs/Cargo.toml" ]; then
	T0=$(date +%s)
	(cd "$W/rs" && cargo test --release --offline --no-run) >"$L/rs-build.log" 2>&1
	rc=$?
	echo "PROBE rust build rc=$rc in $(($(date +%s) - T0)) s"
	if [ $rc -eq 0 ]; then
		rust=built
		grep -E '^warning' "$L/rs-build.log" | sort | uniq -c | head -20 | sed 's/^/  rs-build: /'
	else
		rust=broken
		grep -E -A6 '^error' "$L/rs-build.log" | head -80 | cut -c1-220 | sed 's/^/  rs-build: /'
	fi
fi

# rs FILTER: runs the Rust tests whose names contain FILTER; echoes ok|fail|none and prints their output lines.
rs() {
	[ "$rust" = built ] || { echo "$rust"; return; }
	f=$1
	(cd "$W/rs" && cargo test --release --offline -q "$f" -- --nocapture --test-threads=2) >"$L/rs-$f.log" 2>&1
	r=$?
	grep -E -o '(G[0-9]+[ :]|VALKEY|registry:).*' "$L/rs-$f.log" | cut -c1-260 | sed "s/^/  rs $f: /" >&2
	n=$(grep -E -o 'test result: ok\. [0-9]+ passed' "$L/rs-$f.log" | awk '{s+=$4} END {print s+0}')
	if [ $r -ne 0 ]; then
		grep -E -B2 -A12 'panicked|FAILED|failures:' "$L/rs-$f.log" | head -60 | cut -c1-220 | sed "s/^/  rs $f: /" >&2
		echo fail
	elif [ "$n" -eq 0 ]; then
		echo none
	else
		echo "ok:$n"
	fi
}

# ---- Go
golang=absent
if command -v go >/dev/null 2>&1 && [ -f "$W/go/go.mod" ]; then
	T0=$(date +%s)
	(cd "$W/go" && go vet ./... && go test -count=1 -run '^$' ./...) >"$L/go-build.log" 2>&1
	rc=$?
	echo "PROBE go build+vet rc=$rc in $(($(date +%s) - T0)) s"
	if [ $rc -eq 0 ]; then golang=built; else
		golang=broken
		head -60 "$L/go-build.log" | cut -c1-220 | sed 's/^/  go-build: /'
	fi
	gofmt -l "$W/go" >"$L/gofmt.log" 2>&1
	[ -s "$L/gofmt.log" ] && sed 's/^/  gofmt would change: /' "$L/gofmt.log"
fi

# gt PATTERN: runs the Go tests matching PATTERN; echoes ok|fail|none.
gt() {
	[ "$golang" = built ] || { echo "$golang"; return; }
	p=$1
	(cd "$W/go" && go test -count=1 -v -run "$p" ./...) >"$L/go-$p.log" 2>&1
	r=$?
	grep -E -o '(G[0-9]+[ :]|registry:).*' "$L/go-$p.log" | cut -c1-260 | sed "s/^/  go $p: /" >&2
	n=$(grep -c -- '^--- PASS' "$L/go-$p.log")
	if [ $r -ne 0 ]; then
		grep -E -B2 -A10 -- '--- FAIL|panic:' "$L/go-$p.log" | head -50 | cut -c1-220 | sed "s/^/  go $p: /" >&2
		echo fail
	elif [ "$n" -eq 0 ]; then
		echo none
	else
		echo "ok:$n"
	fi
}

# gate KEY RUST_FILTER GO_PATTERN DESCRIPTION
gate() {
	a=$(rs "$2")
	b=$(gt "$3")
	res=PASS
	case "$a" in ok:*) ;; *) res=FAIL ;; esac
	case "$b" in ok:* | absent) ;; *) res=FAIL ;; esac
	[ "$b" = absent ] && note="; go absent" || note=""
	row "$1" $res "rust=$a go=$b$note: $4"
	[ "$b" = absent ] && cnm=1
}

gate eid-vectors g1_ '^TestG01' "both ways, refusals with reasons, leases, mint scenarios"
gate eid-order g2_ '^TestG02' "1e6 ids text order = byte order; control a-z2-7 breaks it"
gate eid-burst g3_ '^TestG03' "8 leases 1e7 ids unique and rising; borrow; two kinds; control one lease two minters"
gate eid-stream g4_ '^TestG04' "two systems one writer one ms distinct, ordered, inverse; control 48-bit tail; door ahead and kind"
gate eid-restart g5_ '^TestG05' "1000 restarts no live writer reissued; control without the wait repeats"
gate eid-clock-back g6_ '^TestG06' "restore + correct clock no repeat; clock behind refused; restore + clock back counted"
gate eid-fenced-clone g7_ '^TestG07' "other home and second activation refused; pid guard; controls counted"
gate eid-mxdb g9_ '^TestG09' "owner = <code>_sys_ id; sto and dbs; all-zero refused; no time-0 mint"
gate eid-door g10_ '^TestG10' "detached bound as command token, never stored as own; control door off"
gate eid-one-case g11_ '^TestG11' "canonical text in the vectors is one case; uppercase refused"
gate eid-registry registry_ '^TestRegistry' "registry files load; faults refused"
gate eid-registry-retired registry_retired '^TestRegistryRetired' "every kind row of a retired system is retired; ecd sto present and live"
gate eid-brd14 g14_ '^TestG14' "vectors/brd14.tsv both ways, pure and registry layer, every reason; order pairwise; 1e5 random ids return"
gate eid-detached-draw detached_draw '^TestDetachedDraw' "one 16-bit draw, r >= 61440 rejected, code 1024 + r mod 5120; Rust and Go agree; 64 rejections refused"

# gogate KEY GO_PATTERN DESCRIPTION: a gate that only the Go module has.
gogate() {
	b=$(gt "$2")
	case "$b" in ok:*) row "$1" PASS "go=$b: $3" ;; *) row "$1" FAIL "go=$b: $3" ;; esac
}
gogate eid-store '^TestG12' "FileStore: record on disk, one activation per system, 2 processes x 1000 grants give 2000 distinct writers; control without the lock repeats"
gogate eid-cli '^TestG13' "eid against vectors/cli.tsv in process; stores take a lease per call; refusals; detached"
gogate eid-lease '^TestLease' "TakeLease: one writer per call inside one second, the activation released on return, refusals, active and clock-behind; control without the start-wait repeats a writer"
gogate eid-encodings '^TestEncoding' "MarshalText/UnmarshalText, Value/Scan, NullID with JSON; the zero id refused"

# The violating case of eid-brd14: a scratch copy of go/ passes G14 unmodified, and fails it with writer and count
# swapped in FromBrd14.
if [ "$golang" = built ]; then
	M=$(mktemp -d /tmp/eid-brd14-mutant.XXXXXX)
	cp -r "$W/go" "$W/vectors" "$W/registry" "$M/"
	(cd "$M/go" && go test -count=1 -run '^TestG14' .) >"$L/brd14-unmodified.log" 2>&1
	crc=$?
	sed -i 's/uint32(snow>>12)&1023, uint32(snow)&4095/uint32(snow)\&4095, uint32(snow>>12)\&1023/' "$M/go/brd14.go"
	changed=$(cmp -l "$W/go/brd14.go" "$M/go/brd14.go" 2>/dev/null | wc -l)
	(cd "$M/go" && go test -count=1 -run '^TestG14' .) >"$L/brd14-mutant.log" 2>&1
	mrc=$?
	grep -E -m4 -- '--- FAIL|brd14' "$L/brd14-mutant.log" | cut -c1-220 | sed 's/^/  brd14 mutant: /'
	if [ $crc -eq 0 ] && [ "$changed" -gt 0 ] && [ $mrc -ne 0 ]; then
		row eid-brd14-control PASS "go: the unmodified scratch copy passes G14; writer and count swapped in FromBrd14 ($changed bytes): G14 fails, rc=$mrc"
	else
		row eid-brd14-control FAIL "unmodified rc=$crc, bytes changed=$changed, mutant rc=$mrc"
	fi
	rm -rf "$M"
else
	row eid-brd14-control FAIL "go=$golang"
fi

# The violating case of eid-encodings: a scratch copy of go/ passes the encodings unmodified, and fails them with the
# zero refusal of MarshalText removed (the one line "if id == (ID{}) {" becomes "if false {").
if [ "$golang" = built ]; then
	M=$(mktemp -d /tmp/eid-encodings-mutant.XXXXXX)
	cp -r "$W/go" "$W/vectors" "$M/"
	(cd "$M/go" && go test -count=1 -v -run '^TestEncoding' .) >"$L/encodings-unmodified.log" 2>&1
	crc=$?
	sed -i 's/if id == (ID{}) {/if false {/' "$M/go/encoding.go"
	changed=$(cmp -l "$W/go/encoding.go" "$M/go/encoding.go" 2>/dev/null | wc -l)
	(cd "$M/go" && go test -count=1 -v -run '^TestEncoding' .) >"$L/encodings-mutant.log" 2>&1
	mrc=$?
	grep -E -m4 -- '--- FAIL|zero' "$L/encodings-mutant.log" | cut -c1-220 | sed 's/^/  encodings mutant: /'
	if [ $crc -eq 0 ] && [ "$changed" -gt 0 ] && [ $mrc -ne 0 ] && grep -q -- '^--- FAIL: TestEncodingZeroRefused' "$L/encodings-mutant.log"; then
		row eid-encodings-control PASS "go: the unmodified scratch copy passes; the zero refusal of MarshalText removed ($changed differing byte positions): TestEncodingZeroRefused fails, rc=$mrc"
	else
		row eid-encodings-control FAIL "unmodified rc=$crc, bytes changed=$changed, mutant rc=$mrc"
	fi
	rm -rf "$M"
else
	row eid-encodings-control FAIL "go=$golang"
fi

# The violating case of the roles: a scratch copy of go/ passes the role tests unmodified, and fails them with the role
# test of KindsByRole removed (the one line "if row.Role == role {" becomes "if true {").
if [ "$golang" = built ]; then
	M=$(mktemp -d /tmp/eid-roles-mutant.XXXXXX)
	cp -r "$W/go" "$W/vectors" "$W/registry" "$M/"
	(cd "$M/go" && go test -count=1 -v -run '^TestRegistry(Roles|KindsByRole)' .) >"$L/roles-unmodified.log" 2>&1
	crc=$?
	sed -i 's/if row.Role == role {/if true {/' "$M/go/registry.go"
	changed=$(diff "$W/go/registry.go" "$M/go/registry.go" | grep -c '^>')
	(cd "$M/go" && go test -count=1 -v -run '^TestRegistry(Roles|KindsByRole)' .) >"$L/roles-mutant.log" 2>&1
	mrc=$?
	grep -E -m4 -- '--- FAIL|want' "$L/roles-mutant.log" | cut -c1-220 | sed 's/^/  roles mutant: /'
	if [ $crc -eq 0 ] && [ "$changed" -eq 1 ] && [ $mrc -ne 0 ] && grep -q -- '^--- FAIL: TestRegistryKindsByRole ' "$L/roles-mutant.log"; then
		row eid-registry-roles-control PASS "go: the unmodified scratch copy passes the role tests; the role test of KindsByRole removed (1 line): TestRegistryKindsByRole fails, rc=$mrc"
	else
		row eid-registry-roles-control FAIL "unmodified rc=$crc, lines changed=$changed, mutant rc=$mrc"
	fi
	rm -rf "$M"
else
	row eid-registry-roles-control FAIL "go=$golang"
fi

# The violating case of eid-registry-retired: a scratch copy passes its tests in Go and Rust unmodified, and fails them
# with one kind of the retired system brd, crs, turned back to live.
if [ "$golang" = built ] && [ "$rust" = built ]; then
	M=$(mktemp -d /tmp/eid-registry-retired-mutant.XXXXXX)
	cp -r "$W/go" "$W/rs" "$W/registry" "$W/vectors" "$M/"
	# regtests TAG: the eid-registry-retired tests of both languages in the scratch copy; echoes "go=RC rust=RC"
	regtests() {
		(cd "$M/go" && go test -count=1 -v -run '^TestRegistryRetired' .) >"$L/registry-$1-go.log" 2>&1
		g=$?
		(cd "$M/rs" && cargo test --release --offline -q registry_retired -- --test-threads=2) >"$L/registry-$1-rs.log" 2>&1
		echo "go=$g rust=$?"
	}
	before=$(regtests unmodified)
	sed -i 's/^brd\tcrs\tentity\tretired\t/brd\tcrs\tentity\tlive\t/' "$M/registry/kinds.tsv"
	changed=$(diff "$W/registry/kinds.tsv" "$M/registry/kinds.tsv" | grep -c '^>')
	after=$(regtests mutant)
	grep -E -- '--- FAIL|not retired' "$L/registry-mutant-go.log" | head -4 | cut -c1-220 | sed 's/^/  registry mutant go: /'
	grep -E 'panicked|not retired|FAILED' "$L/registry-mutant-rs.log" | head -4 | cut -c1-220 | sed 's/^/  registry mutant rs: /'
	if [ "$before" = "go=0 rust=0" ] && [ "$changed" -eq 1 ] && [ "${after%% *}" != go=0 ] && [ "${after##* }" != rust=0 ]; then
		row eid-registry-retired-control PASS "the unmodified scratch copy passes ($before); brd crs turned back to live (1 line): $after, both red"
	else
		row eid-registry-retired-control FAIL "unmodified $before, lines changed=$changed, mutant $after"
	fi
	rm -rf "$M"
else
	row eid-registry-retired-control FAIL "go=$golang rust=$rust"
fi

# ---- the binaries
if [ "$golang" = built ]; then
	B=/tmp/eid-bin
	rm -rf "$B"
	mkdir -p "$B"
	binok=1
	for t in linux-arm64 darwin-arm64 linux-amd64; do
		for pass in a b; do
			(cd "$W/go" && CGO_ENABLED=0 GOOS=${t%-*} GOARCH=${t#*-} go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$B/$t.$pass" ./cmd/eid) >"$L/bin-$t-$pass.log" 2>&1 || {
				binok=0
				sed "s/^/  bin $t: /" "$L/bin-$t-$pass.log"
			}
		done
		if [ -f "$B/$t.a" ] && [ -f "$B/$t.b" ] && cmp -s "$B/$t.a" "$B/$t.b"; then
			echo "PROBE bin $t sha256 $(sha256sum "$B/$t.a" | cut -d' ' -f1) size $(stat -c %s "$B/$t.a")"
		else
			binok=0
			echo "  bin $t: the two builds differ or one is missing"
		fi
	done
	if [ $binok -eq 1 ]; then
		(cd "$W/go" && EID_BIN="$B/linux-amd64.a" go test -count=1 -v -run '^TestG13_BinaryAgainstVectors$' ./cmd/eid) >"$L/bin-vectors.log" 2>&1
		grep -E 'G13 binary' "$L/bin-vectors.log" | sed 's/^/  bin: /'
		grep -q -- '^--- PASS: TestG13_BinaryAgainstVectors' "$L/bin-vectors.log" || {
			binok=0
			head -20 "$L/bin-vectors.log" | cut -c1-220 | sed 's/^/  bin vectors: /'
		}
	fi
	if [ $binok -eq 1 ]; then
		sum_arm=$(sha256sum "$B/linux-arm64.a" | cut -d' ' -f1)
		sum_mac=$(sha256sum "$B/darwin-arm64.a" | cut -d' ' -f1)
		sum_amd=$(sha256sum "$B/linux-amd64.a" | cut -d' ' -f1)
		for t in linux-arm64 darwin-arm64 linux-amd64; do
			echo "BIN-BEGIN $t sha256=$(sha256sum "$B/$t.a" | cut -d' ' -f1) size=$(stat -c %s "$B/$t.a")"
			gzip -9n -c "$B/$t.a" | base64
			echo "BIN-END $t"
		done
		row eid-bin PASS "linux-arm64 sha256=$sum_arm darwin-arm64 sha256=$sum_mac linux-amd64 sha256=$sum_amd; each built twice with one hash; vectors/cli.tsv agree with the linux-amd64 build"
	else
		row eid-bin FAIL "a build failed, two builds differ, or the built binary disagrees with vectors/cli.tsv"
	fi
else
	row eid-bin FAIL "go=$golang"
fi

echo "ROW COULD-NOT-MEASURE [eid-casefold] a case-folding file system (NTFS, APFS) is not available in a Linux container; eid-one-case asserts one-case text instead; rows/eid-casefold.sh measures it on such a host"

# ---- G4 against a real Valkey
SERVER=""
for s in /opt/ech/valkey/*/bin/valkey-server $(command -v valkey-server 2>/dev/null); do
	[ -x "$s" ] && { SERVER=$s; break; }
done
if [ -z "$SERVER" ]; then
	echo "ROW COULD-NOT-MEASURE [eid-valkey-live] no valkey-server in the image; eid-stream checks Valkey's documented XADD rule"
	cnm=1
elif [ "$rust" != built ]; then
	row eid-valkey-live FAIL "rust=$rust"
else
	"$SERVER" --port 6399 --bind 127.0.0.1 --save '' --appendonly no --daemonize yes --logfile "$L/valkey.log" --pidfile "$L/valkey.pid" >/dev/null 2>&1
	sleep 1
	(cd "$W/rs" && EID_VALKEY=127.0.0.1:6399 cargo test --release --offline -q --test valkey -- --nocapture) >"$L/rs-valkey.log" 2>&1
	r=$?
	grep -E 'VALKEY' "$L/rs-valkey.log" | cut -c1-300 | sed 's/^/  rs valkey: /'
	if [ $r -eq 0 ] && grep -q 'VALKEY-RAN' "$L/rs-valkey.log"; then
		row eid-valkey-live PASS "server $SERVER"
	else
		grep -E -B2 -A12 'panicked|FAILED' "$L/rs-valkey.log" | head -40 | cut -c1-220 | sed 's/^/  rs valkey: /'
		tail -5 "$L/valkey.log" 2>/dev/null | sed 's/^/  valkey.log: /'
		row eid-valkey-live FAIL "rc=$r"
	fi
	[ -f "$L/valkey.pid" ] && kill "$(cat "$L/valkey.pid")" 2>/dev/null
fi

if [ $fail -ne 0 ]; then
	echo "RESULT FAIL"
	exit 1
elif [ $cnm -ne 0 ]; then
	echo "RESULT COULD-NOT-MEASURE (no FAIL; see the rows above)"
	exit 2
fi
echo "RESULT PASS (eid-casefold could-not-measure, as expected in a Linux container)"
exit 0
