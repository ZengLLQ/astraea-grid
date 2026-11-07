package eds

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

// DataAvailabilityHeader (DAH) commits to the 2D Extended Data Square.
// It consists of row NMT roots, column NMT roots, and a root-of-roots data commitment.
type DataAvailabilityHeader struct {
	RowRoots    []nmt.Node
	ColumnRoots []nmt.Node
	DataRoot    [32]byte
}

// ComputeDAH computes the DataAvailabilityHeader for an ExtendedDataSquare.
func ComputeDAH(eds *ExtendedDataSquare) (*DataAvailabilityHeader, error) {
	n := eds.ExtendedWidth
	rowRoots := make([]nmt.Node, n)
	colRoots := make([]nmt.Node, n)

	// Compute row roots using NMT
	for r := 0; r < n; r++ {
		tree := nmt.NewNamespacedMerkleTree()
		for c := 0; c < n; c++ {
			sh := eds.Matrix[r][c]
			_ = tree.Push(nmt.NewNamespacedLeaf(sh.NamespaceID, sh.Data))
		}
		root, err := tree.Root()
		if err != nil {
			return nil, err
		}
		rowRoots[r] = *root
	}

	// Compute column roots using NMT
	for c := 0; c < n; c++ {
		tree := nmt.NewNamespacedMerkleTree()
		for r := 0; r < n; r++ {
			sh := eds.Matrix[r][c]
			_ = tree.Push(nmt.NewNamespacedLeaf(sh.NamespaceID, sh.Data))
		}
		root, err := tree.Root()
		if err != nil {
			return nil, err
		}
		colRoots[c] = *root
	}

	// Combine all row roots and column roots into top-level Merkle DataRoot
	hasher := sha256.New()
	hasher.Write([]byte{0x02}) // DAH commitment domain separator

	for _, rr := range rowRoots {
		digest := rr.Digest()
		hasher.Write(digest[:])
	}
	for _, cr := range colRoots {
		digest := cr.Digest()
		hasher.Write(digest[:])
	}

	var dataRoot [32]byte
	copy(dataRoot[:], hasher.Sum(nil))

	return &DataAvailabilityHeader{
		RowRoots:    rowRoots,
		ColumnRoots: colRoots,
		DataRoot:    dataRoot,
	}, nil
}

func (dah *DataAvailabilityHeader) Hex() string {
	return hex.EncodeToString(dah.DataRoot[:])
}
