package fraud

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
	"github.com/ZengLLQ/astraea-grid/pkg/rs"
)

var (
	ErrInvalidFraudProof = errors.New("fraud proof verification failed")
	ErrEncodingIsCorrect = errors.New("encoding is valid: no fraud detected")
)

// BadEncodingFraudProof (BEFP) proves that a block producer committed to an invalid 2D Reed-Solomon row or column.
type BadEncodingFraudProof struct {
	IsRow            bool                  // true if row, false if column
	Index            int                   // row or column index (0 .. 2k-1)
	OriginalWidth    int                   // k
	DataShares       []eds.Share           // k original data shares
	DataProofs       []*nmt.Proof          // Merkle proofs for each of the k data shares
	ClaimedBadIndex  int                   // column or row index of the bad parity share
	ClaimedBadShare  eds.Share             // the corrupted share committed in the DAH
	ClaimedBadProof  *nmt.Proof            // Merkle proof for the claimed bad share against DAH
}

// CreateBEFP constructs a Bad Encoding Fraud Proof for row r with corrupted parity share at badCol.
func CreateBEFP(
	square *eds.ExtendedDataSquare,
	dah *eds.DataAvailabilityHeader,
	isRow bool,
	index int,
	badIndex int,
) (*BadEncodingFraudProof, error) {
	k := square.OriginalWidth
	n := square.ExtendedWidth

	if index < 0 || index >= n || badIndex < k || badIndex >= n {
		return nil, errors.New("indices out of valid range")
	}

	// 1. Gather the k original data shares and generate their NMT proofs
	dataShares := make([]eds.Share, k)
	dataProofs := make([]*nmt.Proof, k)

	// Build NMT tree for the row or column
	tree := nmt.NewNamespacedMerkleTree()
	for i := 0; i < n; i++ {
		var sh eds.Share
		if isRow {
			sh = square.Matrix[index][i]
		} else {
			sh = square.Matrix[i][index]
		}
		_ = tree.Push(nmt.NewNamespacedLeaf(sh.NamespaceID, sh.Data))
	}

	for i := 0; i < k; i++ {
		if isRow {
			dataShares[i] = square.Matrix[index][i]
		} else {
			dataShares[i] = square.Matrix[i][index]
		}
		proof, err := tree.ProveRange(i, i+1)
		if err != nil {
			return nil, err
		}
		dataProofs[i] = proof
	}

	// 2. Extract the bad parity share and proof
	var badShare eds.Share
	if isRow {
		badShare = square.Matrix[index][badIndex]
	} else {
		badShare = square.Matrix[badIndex][index]
	}

	badProof, err := tree.ProveRange(badIndex, badIndex+1)
	if err != nil {
		return nil, err
	}

	return &BadEncodingFraudProof{
		IsRow:           isRow,
		Index:           index,
		OriginalWidth:   k,
		DataShares:      dataShares,
		DataProofs:      dataProofs,
		ClaimedBadIndex: badIndex,
		ClaimedBadShare: badShare,
		ClaimedBadProof: badProof,
	}, nil
}

// Verify verifies the Bad Encoding Fraud Proof against the trusted DataAvailabilityHeader.
// Returns true if fraud is confirmed (the committed share is indeed invalid).
func (befp *BadEncodingFraudProof) Verify(dah *eds.DataAvailabilityHeader) (bool, error) {
	if dah == nil {
		return false, errors.New("DAH is nil")
	}

	k := befp.OriginalWidth
	if len(befp.DataShares) != k || len(befp.DataProofs) != k {
		return false, errors.New("data share or proof count does not match k")
	}

	var targetRoot *nmt.Node
	if befp.IsRow {
		if befp.Index < 0 || befp.Index >= len(dah.RowRoots) {
			return false, errors.New("row index out of bounds")
		}
		targetRoot = &dah.RowRoots[befp.Index]
	} else {
		if befp.Index < 0 || befp.Index >= len(dah.ColumnRoots) {
			return false, errors.New("column index out of bounds")
		}
		targetRoot = &dah.ColumnRoots[befp.Index]
	}

	// 1. Verify that all k data shares were authentically committed in the DAH
	for i := 0; i < k; i++ {
		leaf := nmt.NewNamespacedLeaf(befp.DataShares[i].NamespaceID, befp.DataShares[i].Data)
		valid, err := befp.DataProofs[i].Verify(targetRoot, []nmt.NamespacedLeaf{leaf}, befp.DataShares[i].NamespaceID)
		if err != nil || !valid {
			return false, fmt.Errorf("data share %d Merkle proof invalid: %w", i, err)
		}
	}

	// 2. Verify that the claimed bad parity share was also authentically committed in the DAH
	badLeaf := nmt.NewNamespacedLeaf(befp.ClaimedBadShare.NamespaceID, befp.ClaimedBadShare.Data)
	validBad, err := befp.ClaimedBadProof.Verify(targetRoot, []nmt.NamespacedLeaf{badLeaf}, befp.ClaimedBadShare.NamespaceID)
	if err != nil || !validBad {
		return false, fmt.Errorf("claimed bad share Merkle proof invalid: %w", err)
	}

	// 3. Re-encode the k verified data shares locally using Reed-Solomon
	rsCodec, err := rs.NewReedSolomon(k, k)
	if err != nil {
		return false, err
	}

	rawShards := make([][]byte, 2*k)
	for i := 0; i < k; i++ {
		rawShards[i] = befp.DataShares[i].Bytes()
	}

	if err := rsCodec.Encode(rawShards); err != nil {
		return false, fmt.Errorf("re-encoding failed: %w", err)
	}

	expectedParity := rawShards[befp.ClaimedBadIndex][nmt.NamespaceSize:]

	// 4. Compare expected parity with the committed bad share
	if bytes.Equal(expectedParity, befp.ClaimedBadShare.Data) {
		// The committed share actually matches the correct RS parity -> No fraud!
		return false, ErrEncodingIsCorrect
	}

	// Mismatch confirmed! The block producer committed an invalid share!
	return true, nil
}
