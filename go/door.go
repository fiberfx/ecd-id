package ecdid

// Admission is the result of admitting an id. When Own is true, ID is the system's own id and is stored as such.
// When Own is false, Token was a detached id: the door minted ID and bound Token to it; Token is never stored as a
// fact.
type Admission struct {
	Own   bool
	Token ID
	ID    ID
}

// DefaultAheadMs is the default limit on how far ahead of the door's clock an id may be stamped: 5 seconds.
const DefaultAheadMs = 5_000

// Door is the door of one registered system (spec/lease.md#door).
type Door struct {
	minter  *Minter
	kinds   map[Code]bool
	aheadMs uint64
	clock   Clock
	tokens  map[ID]ID
}

// NewDoor returns a door that mints with minter (its lease names the door's system), accepts kinds, and refuses ids
// stamped more than aheadMs milliseconds ahead of clock (DefaultAheadMs unless the system sets another).
func NewDoor(minter *Minter, kinds []Code, aheadMs uint64, clock Clock) *Door {
	d := &Door{minter: minter, kinds: map[Code]bool{}, aheadMs: aheadMs, clock: clock, tokens: map[ID]ID{}}
	for _, k := range kinds {
		d.kinds[k] = true
	}
	return d
}

// System returns the door's system.
func (d *Door) System() Code { return d.minter.Lease().System }

// Admit admits id. lease is the lease this system issued to the sending session, or nil when it issued none.
func (d *Door) Admit(id ID, lease *Lease) (Admission, error) {
	if !d.kinds[id.Kind()] {
		return Admission{}, ErrKind
	}
	if id.Class() == Detached {
		if own, ok := d.tokens[id]; ok {
			return Admission{Token: id, ID: own}, nil
		}
		own, err := d.minter.Mint(id.Kind())
		if err != nil {
			return Admission{}, err
		}
		d.tokens[id] = own
		return Admission{Token: id, ID: own}, nil
	}
	if id.System() != d.System() {
		return Admission{}, ErrForeign
	}
	if id.Time() > d.clock.NowMs()+d.aheadMs {
		return Admission{}, ErrAhead
	}
	if lease != nil && (lease.System != id.System() || lease.Writer != id.Writer() ||
		id.Time() < lease.NotBefore || id.Time() > lease.NotAfter) {
		return Admission{}, ErrLease
	}
	return Admission{Own: true, ID: id}, nil
}

// Bound returns the number of detached ids bound as command tokens.
func (d *Door) Bound() int { return len(d.tokens) }
