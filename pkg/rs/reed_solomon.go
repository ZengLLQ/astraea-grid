package rs

import (
	"errors"
	"fmt"
)

// ReedSolomon provides systematic Reed-Solomon erasure coding.
type ReedSolomon struct {
	DataShards   int
	ParityShards int
	TotalShards  int
	GenMatrix    *Matrix // TotalShards x DataShards systematic generator matrix
	ParityMatrix *Matrix // ParityShards x DataShards Cauchy matrix
	gf           *GF256
}

// NewReedSolomon creates a new Reed-Solomon encoder/decoder.
func NewReedSolomon(dataShards, parityShards int) (*ReedSolomon, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, errors.New("data and parity shard counts must be positive")
	}
	total := dataShards + parityShards
	if total > 256 {
		return nil, fmt.Errorf("total shards %d exceeds GF(256) field size 256", total)
	}

	// Systematic Generator Matrix G = [ I_k ; Cauchy_m_k ]
	gen := NewMatrix(total, dataShards)
	for i := 0; i < dataShards; i++ {
		gen.Data[i][i] = 1
	}

	cauchy := CauchyMatrix(parityShards, dataShards)
	for i := 0; i < parityShards; i++ {
		copy(gen.Data[dataShards+i], cauchy.Data[i])
	}

	return &ReedSolomon{
		DataShards:   dataShards,
		ParityShards: parityShards,
		TotalShards:  total,
		GenMatrix:    gen,
		ParityMatrix: cauchy,
		gf:           DefaultGF256,
	}, nil
}

// Encode computes parity shards from data shards.
// Input slice must have length TotalShards, with first DataShards populated.
func (rs *ReedSolomon) Encode(shards [][]byte) error {
	if len(shards) != rs.TotalShards {
		return fmt.Errorf("expected %d shards, got %d", rs.TotalShards, len(shards))
	}

	shardSize := len(shards[0])
	if shardSize == 0 {
		return errors.New("shard size cannot be zero")
	}

	// Check data shards
	for i := 0; i < rs.DataShards; i++ {
		if len(shards[i]) != shardSize {
			return fmt.Errorf("data shard %d has mismatched size %d != %d", i, len(shards[i]), shardSize)
		}
	}

	// Allocate parity shards if needed
	for i := rs.DataShards; i < rs.TotalShards; i++ {
		if len(shards[i]) != shardSize {
			shards[i] = make([]byte, shardSize)
		} else {
			// Clear existing buffer
			for j := range shards[i] {
				shards[i][j] = 0
			}
		}
	}

	// Compute parity: P[i] = sum_j (ParityMatrix[i][j] * Data[j])
	for i := 0; i < rs.ParityShards; i++ {
		parityIdx := rs.DataShards + i
		coeffRow := rs.ParityMatrix.Data[i]
		for j := 0; j < rs.DataShards; j++ {
			c := coeffRow[j]
			rs.gf.AddMulSlice(c, shards[j], shards[parityIdx])
		}
	}

	return nil
}

// Verify checks whether parity shards correctly match the data shards.
func (rs *ReedSolomon) Verify(shards [][]byte) (bool, error) {
	if len(shards) != rs.TotalShards {
		return false, fmt.Errorf("expected %d shards, got %d", rs.TotalShards, len(shards))
	}
	shardSize := len(shards[0])
	for i := 0; i < rs.TotalShards; i++ {
		if len(shards[i]) != shardSize {
			return false, fmt.Errorf("shard %d has mismatched length", i)
		}
	}

	// Recompute expected parity in temporary buffers
	tempParity := make([][]byte, rs.ParityShards)
	for i := range tempParity {
		tempParity[i] = make([]byte, shardSize)
	}

	for i := 0; i < rs.ParityShards; i++ {
		coeffRow := rs.ParityMatrix.Data[i]
		for j := 0; j < rs.DataShards; j++ {
			rs.gf.AddMulSlice(coeffRow[j], shards[j], tempParity[i])
		}
		// Compare with given parity
		actualParity := shards[rs.DataShards+i]
		for b := 0; b < shardSize; b++ {
			if tempParity[i][b] != actualParity[b] {
				return false, nil
			}
		}
	}

	return true, nil
}

// Reconstruct recovers missing shards if at least DataShards shards are present.
// Missing shards must be nil or empty slices.
func (rs *ReedSolomon) Reconstruct(shards [][]byte) error {
	if len(shards) != rs.TotalShards {
		return fmt.Errorf("expected %d shards, got %d", rs.TotalShards, len(shards))
	}

	// Identify shard size and missing shards
	shardSize := 0
	numPresent := 0
	submatrixRows := make([]int, 0, rs.DataShards)

	for i := 0; i < rs.TotalShards; i++ {
		if len(shards[i]) > 0 {
			if shardSize == 0 {
				shardSize = len(shards[i])
			} else if len(shards[i]) != shardSize {
				return fmt.Errorf("shard %d has mismatched size", i)
			}
			if numPresent < rs.DataShards {
				submatrixRows = append(submatrixRows, i)
			}
			numPresent++
		}
	}

	if numPresent < rs.DataShards {
		return fmt.Errorf("%w: have %d, require %d", ErrInsufficientShards, numPresent, rs.DataShards)
	}

	// Check if any shard needs reconstruction
	missingAny := false
	for i := 0; i < rs.TotalShards; i++ {
		if len(shards[i]) == 0 {
			missingAny = true
			break
		}
	}
	if !missingAny {
		return nil // Everything is already present
	}

	// Extract k x k submatrix from GenMatrix corresponding to the k available shards
	submatrix := NewMatrix(rs.DataShards, rs.DataShards)
	for r, rowIdx := range submatrixRows {
		copy(submatrix.Data[r], rs.GenMatrix.Data[rowIdx])
	}

	// Invert the submatrix
	invMatrix, err := submatrix.Invert()
	if err != nil {
		return fmt.Errorf("failed to invert submatrix: %w", err)
	}

	// Reconstruct the original k data shards: Data = invMatrix * AvailableShards
	recoveredData := make([][]byte, rs.DataShards)
	for i := 0; i < rs.DataShards; i++ {
		recoveredData[i] = make([]byte, shardSize)
		for j := 0; j < rs.DataShards; j++ {
			coeff := invMatrix.Data[i][j]
			srcShard := shards[submatrixRows[j]]
			rs.gf.AddMulSlice(coeff, srcShard, recoveredData[i])
		}
	}

	// Populate any missing data shards
	for i := 0; i < rs.DataShards; i++ {
		if len(shards[i]) == 0 {
			shards[i] = recoveredData[i]
		}
	}

	// Re-encode all parity shards to fill in any missing parity
	for i := 0; i < rs.ParityShards; i++ {
		parityIdx := rs.DataShards + i
		if len(shards[parityIdx]) == 0 {
			shards[parityIdx] = make([]byte, shardSize)
			coeffRow := rs.ParityMatrix.Data[i]
			for j := 0; j < rs.DataShards; j++ {
				rs.gf.AddMulSlice(coeffRow[j], shards[j], shards[parityIdx])
			}
		}
	}

	return nil
}
