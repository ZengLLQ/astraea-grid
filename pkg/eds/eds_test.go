package eds

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

func generateTestDataSquare(k, shareSize int) [][]Share {
	dataSquare := make([][]Share, k)
	for r := 0; r < k; r++ {
		dataSquare[r] = make([]Share, k)
		for c := 0; c < k; c++ {
			nid := nmt.NewNamespaceID(uint64(r*k + c + 1))
			data := make([]byte, shareSize)
			for b := range data {
				data[b] = byte((r*k + c + b) % 255)
			}
			dataSquare[r][c] = NewShare(nid, data)
		}
	}
	return dataSquare
}

func TestEDS2DEncodingAndRoots(t *testing.T) {
	k := 4
	shareSize := 32
	dataSquare := generateTestDataSquare(k, shareSize)

	eds, err := NewExtendedDataSquare(dataSquare)
	if err != nil {
		t.Fatalf("failed to create EDS: %v", err)
	}

	if eds.OriginalWidth != 4 || eds.ExtendedWidth != 8 {
		t.Fatalf("unexpected dimensions: %dx%d", eds.OriginalWidth, eds.ExtendedWidth)
	}

	// Verify Quadrant 1 matches input
	for r := 0; r < k; r++ {
		for c := 0; c < k; c++ {
			if !bytes.Equal(eds.Matrix[r][c].Data, dataSquare[r][c].Data) {
				t.Fatalf("Quadrant 1 mismatch at (%d, %d)", r, c)
			}
		}
	}

	// Verify parity namespaces
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if r >= k || c >= k {
				if !eds.Matrix[r][c].NamespaceID.Equal(nmt.ParityNamespace) {
					t.Fatalf("expected parity namespace at (%d, %d), got %s", r, c, eds.Matrix[r][c].NamespaceID)
				}
			}
		}
	}

	// Compute DAH
	dah, err := ComputeDAH(eds)
	if err != nil {
		t.Fatalf("failed to compute DAH: %v", err)
	}

	if len(dah.RowRoots) != 8 || len(dah.ColumnRoots) != 8 {
		t.Fatalf("expected 8 row and column roots, got %d and %d", len(dah.RowRoots), len(dah.ColumnRoots))
	}

	fmt.Printf("Data Root: %s\n", dah.Hex())
}

func TestEDSIterativeReconstruction(t *testing.T) {
	k := 4
	shareSize := 32
	dataSquare := generateTestDataSquare(k, shareSize)

	eds, err := NewExtendedDataSquare(dataSquare)
	if err != nil {
		t.Fatalf("failed to create EDS: %v", err)
	}

	// Corrupt EDS by dropping shares (keeping at least k per row/column)
	// Drop 2 shares from each row in Quadrant 1 and 2
	for r := 0; r < eds.ExtendedWidth; r++ {
		eds.Matrix[r][0] = Share{}
		eds.Matrix[r][2] = Share{}
	}

	if err := eds.Reconstruct(); err != nil {
		t.Fatalf("iterative reconstruction failed: %v", err)
	}

	// Verify original data is restored
	for r := 0; r < k; r++ {
		for c := 0; c < k; c++ {
			if !bytes.Equal(eds.Matrix[r][c].Data, dataSquare[r][c].Data) {
				t.Fatalf("reconstructed data mismatch at (%d, %d)", r, c)
			}
		}
	}
}

func TestEDSIrrecoverableDrop(t *testing.T) {
	k := 4
	shareSize := 32
	dataSquare := generateTestDataSquare(k, shareSize)

	eds, _ := NewExtendedDataSquare(dataSquare)

	// Drop more than k shares from all rows and columns (e.g. drop 5 shares per row)
	for r := 0; r < eds.ExtendedWidth; r++ {
		for c := 0; c < 5; c++ {
			eds.Matrix[r][c] = Share{}
		}
	}

	err := eds.Reconstruct()
	if err == nil {
		t.Fatalf("expected reconstruction failure when excessive shares dropped")
	}
}
