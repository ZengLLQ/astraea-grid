package das

import (
	"errors"
	"fmt"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

var (
	ErrInvalidCoordinate = errors.New("sample coordinate out of bounds")
	ErrProofVerificationFailed = errors.New("DAS sample proof verification failed")
)

// Coordinate represents a 2D sample coordinate in the Extended Data Square.
type Coordinate struct {
	Row int
	Col int
}

func (c Coordinate) String() string {
	return fmt.Sprintf("(%d, %d)", c.Row, c.Col)
}

// Sample represents a sampled share together with its row and column NMT inclusion proofs.
type Sample struct {
	Coordinate Coordinate
	Share      eds.Share
	RowProof   *nmt.Proof
	ColProof   *nmt.Proof
}

// CreateSample extracts a share at (row, col) from the EDS and builds its row and column proofs.
func CreateSample(square *eds.ExtendedDataSquare, coord Coordinate) (*Sample, error) {
	n := square.ExtendedWidth
	if coord.Row < 0 || coord.Row >= n || coord.Col < 0 || coord.Col >= n {
		return nil, fmt.Errorf("%w: %s for %dx%d square", ErrInvalidCoordinate, coord, n, n)
	}

	sh := square.Matrix[coord.Row][coord.Col]

	// 1. Build row NMT and generate proof for Col
	rowTree := nmt.NewNamespacedMerkleTree()
	for c := 0; c < n; c++ {
		curShare := square.Matrix[coord.Row][c]
		_ = rowTree.Push(nmt.NewNamespacedLeaf(curShare.NamespaceID, curShare.Data))
	}
	rowProof, err := rowTree.ProveRange(coord.Col, coord.Col+1)
	if err != nil {
		return nil, fmt.Errorf("failed to prove row share: %w", err)
	}

	// 2. Build column NMT and generate proof for Row
	colTree := nmt.NewNamespacedMerkleTree()
	for r := 0; r < n; r++ {
		curShare := square.Matrix[r][coord.Col]
		_ = colTree.Push(nmt.NewNamespacedLeaf(curShare.NamespaceID, curShare.Data))
	}
	colProof, err := colTree.ProveRange(coord.Row, coord.Row+1)
	if err != nil {
		return nil, fmt.Errorf("failed to prove col share: %w", err)
	}

	return &Sample{
		Coordinate: coord,
		Share:      sh,
		RowProof:   rowProof,
		ColProof:   colProof,
	}, nil
}

// Verify verifies the sample's share against the DataAvailabilityHeader.
func (s *Sample) Verify(dah *eds.DataAvailabilityHeader) (bool, error) {
	if dah == nil {
		return false, errors.New("DAH is nil")
	}

	n := len(dah.RowRoots)
	if s.Coordinate.Row < 0 || s.Coordinate.Row >= n || s.Coordinate.Col < 0 || s.Coordinate.Col >= n {
		return false, ErrInvalidCoordinate
	}

	leaf := nmt.NewNamespacedLeaf(s.Share.NamespaceID, s.Share.Data)

	// Verify Row proof against DAH RowRoot
	rowRoot := &dah.RowRoots[s.Coordinate.Row]
	validRow, err := s.RowProof.Verify(rowRoot, []nmt.NamespacedLeaf{leaf}, s.Share.NamespaceID)
	if err != nil || !validRow {
		return false, fmt.Errorf("row proof invalid: %w", err)
	}

	// Verify Col proof against DAH ColRoot
	colRoot := &dah.ColumnRoots[s.Coordinate.Col]
	validCol, err := s.ColProof.Verify(colRoot, []nmt.NamespacedLeaf{leaf}, s.Share.NamespaceID)
	if err != nil || !validCol {
		return false, fmt.Errorf("col proof invalid: %w", err)
	}

	return true, nil
}
