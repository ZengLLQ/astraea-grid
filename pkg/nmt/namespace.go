package nmt

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

const NamespaceSize = 8

// NamespaceID represents an 8-byte namespace identifier.
type NamespaceID [NamespaceSize]byte

var (
	// MinNamespace is the lowest possible namespace ID (reserved for system/header).
	MinNamespace = NamespaceID{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	// TailPaddingNamespace is used to pad incomplete rows/columns before parity.
	TailPaddingNamespace = NamespaceID{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFE}
	// ParityNamespace is reserved for Reed-Solomon parity shares.
	ParityNamespace = NamespaceID{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
)

func NewNamespaceID(val uint64) NamespaceID {
	var nid NamespaceID
	for i := 7; i >= 0; i-- {
		nid[i] = byte(val & 0xFF)
		val >>= 8
	}
	return nid
}

func FromHex(s string) (NamespaceID, error) {
	var nid NamespaceID
	b, err := hex.DecodeString(s)
	if err != nil {
		return nid, err
	}
	if len(b) != NamespaceSize {
		return nid, fmt.Errorf("invalid namespace hex length: got %d, want %d", len(b), NamespaceSize)
	}
	copy(nid[:], b)
	return nid, nil
}

func (nid NamespaceID) Hex() string {
	return hex.EncodeToString(nid[:])
}

func (nid NamespaceID) Compare(other NamespaceID) int {
	return bytes.Compare(nid[:], other[:])
}

func (nid NamespaceID) Less(other NamespaceID) bool {
	return bytes.Compare(nid[:], other[:]) < 0
}

func (nid NamespaceID) Equal(other NamespaceID) bool {
	return bytes.Equal(nid[:], other[:])
}

func (nid NamespaceID) String() string {
	return nid.Hex()
}
