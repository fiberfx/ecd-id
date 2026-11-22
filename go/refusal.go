package eid

// Refusal is an error whose text is the reason the specification and the vectors name.
type Refusal string

func (r Refusal) Error() string { return string(r) }

// The refusals.
const (
	ErrLength         Refusal = "length"
	ErrSeparator      Refusal = "separator"
	ErrAlphabet       Refusal = "alphabet"
	ErrPad            Refusal = "pad"
	ErrReserved       Refusal = "reserved"
	ErrSystemReserved Refusal = "system-reserved"
	ErrKind           Refusal = "kind"
	ErrRange          Refusal = "range"
	ErrStreamSyntax   Refusal = "stream-syntax"
	ErrStreamRange    Refusal = "stream-range"
	ErrLeaseSyntax    Refusal = "lease-syntax"
	ErrLeaseSystem    Refusal = "lease-system"
	ErrLeaseWriter    Refusal = "lease-writer"
	ErrLeaseWindow    Refusal = "lease-window"
	ErrLeaseEnded     Refusal = "lease-ended"
	ErrFork           Refusal = "fork"
	ErrRandom         Refusal = "random"
	ErrWait           Refusal = "wait"
	ErrClockBehind    Refusal = "clock-behind"
	ErrSecondFull     Refusal = "second-full"
	ErrContended      Refusal = "contended"
	ErrClockZero      Refusal = "clock-zero"
	ErrTerm           Refusal = "term"
	ErrClass          Refusal = "class"
	ErrUnknown        Refusal = "unknown"
	ErrState          Refusal = "state"
	ErrNotHome        Refusal = "not-home"
	ErrActive         Refusal = "active"
	ErrForeign        Refusal = "foreign"
	ErrAhead          Refusal = "ahead"
	ErrLease          Refusal = "lease"
)
