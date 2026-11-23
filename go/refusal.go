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
	ErrBrd14Length    Refusal = "brd14-length"
	ErrBrd14Namespace Refusal = "brd14-namespace"
	ErrBrd14Alphabet  Refusal = "brd14-alphabet"
	ErrBrd14Range     Refusal = "brd14-range"
	ErrBrd14System    Refusal = "brd14-system"
	ErrBrd14Time      Refusal = "brd14-time"
	ErrBrd14Writer    Refusal = "brd14-writer"
	ErrBrd14Count     Refusal = "brd14-count"
	ErrBrd14Kind      Refusal = "brd14-kind"
)
