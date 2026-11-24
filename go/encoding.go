package eid

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
)

// Refusals of the text, JSON and SQL encodings, in the form of the refusals of refusal.go.
const (
	ErrZero     Refusal = "zero"
	ErrScanType Refusal = "scan-type"
	ErrJSONType Refusal = "json-type"
)

var zeroID ID

// MarshalText returns the 28-character canonical text. The zero ID names no id and is refused with ErrZero.
func (id ID) MarshalText() ([]byte, error) {
	if id == (ID{}) {
		return nil, ErrZero
	}
	t := id.Encode()
	return t[:], nil
}

// UnmarshalText parses the canonical text with the strict parse. On a refusal *id is unchanged.
func (id *ID) UnmarshalText(b []byte) error {
	v, err := Parse(b)
	if err != nil {
		return err
	}
	*id = v
	return nil
}

// Value returns the 16 bytes as a []byte that the caller owns. The zero ID is refused with ErrZero.
func (id ID) Value() (driver.Value, error) {
	if id == zeroID {
		return nil, ErrZero
	}
	return id.Bytes(), nil
}

// Scan reads an id from a []byte of exactly 16 bytes through FromBytes. Text, nil and any other type are refused with
// ErrScanType, any other length with ErrLength. On a refusal *id is unchanged.
func (id *ID) Scan(src any) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrScanType
	}
	v, err := FromBytes(b)
	if err != nil {
		return err
	}
	*id = v
	return nil
}

// NullID is an ID that may be absent: a nullable column, or a JSON null.
type NullID struct {
	ID    ID
	Valid bool
}

// Value returns nil when n is not valid, otherwise the value of the ID.
func (n NullID) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.ID.Value()
}

// Scan reads nil as an absent id and anything else as ID.Scan does. On a refusal *n is unchanged.
func (n *NullID) Scan(src any) error {
	if src == nil {
		*n = NullID{}
		return nil
	}
	var id ID
	if err := id.Scan(src); err != nil {
		return err
	}
	*n = NullID{ID: id, Valid: true}
	return nil
}

// MarshalJSON returns null when n is not valid, otherwise the JSON string of the canonical text.
func (n NullID) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	t, err := n.ID.MarshalText()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(t)+2)
	out = append(out, '"')
	out = append(out, t...)
	return append(out, '"'), nil
}

// UnmarshalJSON reads null as an absent id and a JSON string through the strict parse; anything else is refused with
// ErrJSONType. On a refusal *n is unchanged.
func (n *NullID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if string(b) == "null" {
		*n = NullID{}
		return nil
	}
	if len(b) == 0 || b[0] != '"' {
		return ErrJSONType
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return ErrJSONType
	}
	id, err := ParseString(s)
	if err != nil {
		return err
	}
	*n = NullID{ID: id, Valid: true}
	return nil
}
