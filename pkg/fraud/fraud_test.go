package fraud

import (
	"testing"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

func makeTestSquare(k int) [][]eds.Share {
	dataSquare := make([][]eds.Share, k)
	for r := 0; r < k; r++ {
		dataSquare[r] = make([]eds.Share, k)
		for c := 0; c < k; c++ {
			nid := nmt.NewNamespaceID(uint64(r*k + c + 1))
			data := []byte{byte(r), byte(c), 0xDE, 0xAD, 0xBE, 0xEF}
			dataSquare[r][c] = eds.NewShare(nid, data)
		}
	}
	return dataSquare
}

func TestBadEncodingFraudProofVerification(t *testing.T) {
	k := 4
	dataSquare := makeTestSquare(k)

	// 1. Generate honestly encoded square
	square, err := eds.NewExtendedDataSquare(dataSquare)
	if err != nil {
		t.Fatalf("failed to create EDS: %v", err)
	}

	// 2. Malicious block producer modifies row 1, parity share 5
	corruptedRow := 1
	badCol := 5
	square.Matrix[corruptedRow][badCol].Data[0] ^= 0x55 // bitflip in committed parity share

	// 3. Block producer computes and publishes DAH committing to the corrupted matrix
	dah, err := eds.ComputeDAH(square)
	if err != nil {
		t.Fatalf("failed to compute DAH: %v", err)
	}

	// 4. Honest full node detects inconsistency and generates BEFP
	befp, err := CreateBEFP(square, dah, true, corruptedRow, badCol)
	if err != nil {
		t.Fatalf("failed to create BEFP: %v", err)
	}

	// 5. Light client or network verifier checks BEFP
	fraudConfirmed, err := befp.Verify(dah)
	if err != nil {
		t.Fatalf("unexpected error during verification: %v", err)
	}
	if !fraudConfirmed {
		t.Fatalf("expected fraud to be confirmed on corrupted parity share")
	}
}

func TestFraudProofOnHonestEncodingRejected(t *testing.T) {
	k := 4
	dataSquare := makeTestSquare(k)

	square, err := eds.NewExtendedDataSquare(dataSquare)
	if err != nil {
		t.Fatalf("failed to create EDS: %v", err)
	}
	dah, err := eds.ComputeDAH(square)
	if err != nil {
		t.Fatalf("failed to compute DAH: %v", err)
	}

	// Try to create a fraud proof against an HONEST share (row 0, col 4)
	befp, err := CreateBEFP(square, dah, true, 0, 4)
	if err != nil {
		t.Fatalf("failed to create BEFP: %v", err)
	}

	// Verifier should reject the claim of fraud because encoding is actually correct
	fraudConfirmed, err := befp.Verify(dah)
	if fraudConfirmed || err != ErrEncodingIsCorrect {
		t.Fatalf("expected ErrEncodingIsCorrect, got confirmed=%v, err=%v", fraudConfirmed, err)
	}
}
