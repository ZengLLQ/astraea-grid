package mempool

import (
	"bytes"
	"testing"

	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

func TestBlobCreationAndReservedNamespace(t *testing.T) {
	validNid := nmt.NewNamespaceID(42)
	b, err := NewBlob(validNid, []byte("rollup-tx-data"), "0xAlice", 1000)
	if err != nil {
		t.Fatalf("unexpected error creating valid blob: %v", err)
	}
	if b.Signer != "0xAlice" || b.Fee != 1000 {
		t.Fatalf("blob metadata mismatch")
	}

	// Reserved namespaces must be rejected
	for _, resNid := range []nmt.NamespaceID{nmt.MinNamespace, nmt.ParityNamespace, nmt.TailPaddingNamespace} {
		_, err := NewBlob(resNid, []byte("data"), "0xBob", 500)
		if err != ErrReservedNamespace {
			t.Fatalf("expected ErrReservedNamespace for %s, got %v", resNid, err)
		}
	}
}

func TestBlobPackerSortingAndPadding(t *testing.T) {
	packer := NewBlobPacker(32)

	// Create blobs with out-of-order namespaces
	b1, _ := NewBlob(nmt.NewNamespaceID(100), []byte("rollup-C"), "0xC", 10)
	b2, _ := NewBlob(nmt.NewNamespaceID(20), []byte("rollup-A"), "0xA", 10)
	b3, _ := NewBlob(nmt.NewNamespaceID(50), []byte("rollup-B"), "0xB", 10)

	blobs := []*Blob{b1, b2, b3}
	grid, err := packer.Pack(blobs)
	if err != nil {
		t.Fatalf("pack failed: %v", err)
	}

	k := len(grid)
	if k != 2 { // 3 shares fit in 2x2 = 4 capacity
		t.Fatalf("expected k=2, got k=%d", k)
	}

	// Verify non-decreasing namespace order across row-major grid
	var flattenedNids []nmt.NamespaceID
	for r := 0; r < k; r++ {
		for c := 0; c < k; c++ {
			flattenedNids = append(flattenedNids, grid[r][c].NamespaceID)
		}
	}

	for i := 1; i < len(flattenedNids); i++ {
		if flattenedNids[i].Less(flattenedNids[i-1]) {
			t.Fatalf("flattened grid namespace out of order at index %d: %s after %s",
				i, flattenedNids[i], flattenedNids[i-1])
		}
	}

	// The 4th share must be TailPaddingNamespace
	if !flattenedNids[3].Equal(nmt.TailPaddingNamespace) {
		t.Fatalf("expected tail padding namespace at slot 3, got %s", flattenedNids[3])
	}
}

func TestPackerLargeBlobFragmentation(t *testing.T) {
	packer := NewBlobPacker(16)
	data := bytes.Repeat([]byte("X"), 60) // 60 bytes / 16 bytes = 4 shares (16 + 16 + 16 + 12)

	b, _ := NewBlob(nmt.NewNamespaceID(77), data, "0xBigRollup", 100)
	grid, err := packer.Pack([]*Blob{b})
	if err != nil {
		t.Fatalf("failed to pack large blob: %v", err)
	}

	k := len(grid)
	if k != 2 {
		t.Fatalf("expected 2x2 grid for 4 shares, got %d", k)
	}

	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			if !grid[r][c].NamespaceID.Equal(nmt.NewNamespaceID(77)) {
				t.Fatalf("expected namespace 77 at (%d, %d)", r, c)
			}
		}
	}
}
