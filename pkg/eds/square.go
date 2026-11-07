package eds

import (
	"errors"
	"fmt"

	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
	"github.com/ZengLLQ/astraea-grid/pkg/rs"
)

var (
	ErrInvalidDimensions = errors.New("square dimensions must be a positive power of 2")
	ErrReconstructionFailed = errors.New("unable to reconstruct EDS: insufficient shares")
)

// Share represents a single data or parity share in the Extended Data Square.
type Share struct {
	NamespaceID nmt.NamespaceID
	Data        []byte
}

func NewShare(nid nmt.NamespaceID, data []byte) Share {
	return Share{
		NamespaceID: nid,
		Data:        data,
	}
}

func (s Share) Bytes() []byte {
	b := make([]byte, nmt.NamespaceSize+len(s.Data))
	copy(b[:nmt.NamespaceSize], s.NamespaceID[:])
	copy(b[nmt.NamespaceSize:], s.Data)
	return b
}

func ShareFromBytes(b []byte) (Share, error) {
	if len(b) < nmt.NamespaceSize {
		return Share{}, fmt.Errorf("share byte slice too short: %d < %d", len(b), nmt.NamespaceSize)
	}
	var nid nmt.NamespaceID
	copy(nid[:], b[:nmt.NamespaceSize])
	data := make([]byte, len(b)-nmt.NamespaceSize)
	copy(data, b[nmt.NamespaceSize:])
	return Share{
		NamespaceID: nid,
		Data:        data,
	}, nil
}

func (s Share) IsEmpty() bool {
	return len(s.Data) == 0 && s.NamespaceID == nmt.NamespaceID{}
}

// ExtendedDataSquare represents a 2k x 2k matrix of shares generated via 2D Reed-Solomon coding.
type ExtendedDataSquare struct {
	OriginalWidth int // k
	ExtendedWidth int // 2k
	ShareSize     int
	Matrix        [][]Share
}

// NewExtendedDataSquare creates and encodes a 2k x 2k EDS from a k x k data matrix.
func NewExtendedDataSquare(dataSquare [][]Share) (*ExtendedDataSquare, error) {
	k := len(dataSquare)
	if k == 0 || (k&(k-1)) != 0 {
		return nil, ErrInvalidDimensions
	}
	for i := range dataSquare {
		if len(dataSquare[i]) != k {
			return nil, fmt.Errorf("row %d has length %d, expected %d", i, len(dataSquare[i]), k)
		}
	}

	shareSize := len(dataSquare[0][0].Data)
	extendedWidth := 2 * k

	// Initialize 2k x 2k matrix
	matrix := make([][]Share, extendedWidth)
	for i := range matrix {
		matrix[i] = make([]Share, extendedWidth)
	}

	// 1. Copy original k x k data into top-left Quadrant 1
	for r := 0; r < k; r++ {
		for c := 0; c < k; c++ {
			matrix[r][c] = dataSquare[r][c]
		}
	}

	rsCodec, err := rs.NewReedSolomon(k, k)
	if err != nil {
		return nil, err
	}

	// 2. Encode rows 0 .. k-1 to compute top-right Quadrant 2
	for r := 0; r < k; r++ {
		rawShards := make([][]byte, extendedWidth)
		for c := 0; c < k; c++ {
			rawShards[c] = matrix[r][c].Bytes()
		}
		if err := rsCodec.Encode(rawShards); err != nil {
			return nil, fmt.Errorf("row %d encode failed: %w", r, err)
		}
		// Populate parity shares with ParityNamespace
		for c := k; c < extendedWidth; c++ {
			matrix[r][c] = Share{
				NamespaceID: nmt.ParityNamespace,
				Data:        rawShards[c][nmt.NamespaceSize:],
			}
		}
	}

	// 3. Encode columns 0 .. 2k-1 to compute bottom Quadrants 3 and 4
	for c := 0; c < extendedWidth; c++ {
		rawShards := make([][]byte, extendedWidth)
		for r := 0; r < k; r++ {
			rawShards[r] = matrix[r][c].Bytes()
		}
		if err := rsCodec.Encode(rawShards); err != nil {
			return nil, fmt.Errorf("column %d encode failed: %w", c, err)
		}
		for r := k; r < extendedWidth; r++ {
			matrix[r][c] = Share{
				NamespaceID: nmt.ParityNamespace,
				Data:        rawShards[r][nmt.NamespaceSize:],
			}
		}
	}

	return &ExtendedDataSquare{
		OriginalWidth: k,
		ExtendedWidth: extendedWidth,
		ShareSize:     shareSize,
		Matrix:        matrix,
	}, nil
}

// Row returns all shares in row r.
func (eds *ExtendedDataSquare) Row(r int) []Share {
	res := make([]Share, eds.ExtendedWidth)
	copy(res, eds.Matrix[r])
	return res
}

// Col returns all shares in column c.
func (eds *ExtendedDataSquare) Col(c int) []Share {
	res := make([]Share, eds.ExtendedWidth)
	for r := 0; r < eds.ExtendedWidth; r++ {
		res[r] = eds.Matrix[r][c]
	}
	return res
}

// Reconstruct recovers missing shares iteratively by alternating row and column RS decoding.
func (eds *ExtendedDataSquare) Reconstruct() error {
	k := eds.OriginalWidth
	n := eds.ExtendedWidth
	rsCodec, err := rs.NewReedSolomon(k, k)
	if err != nil {
		return err
	}

	madeProgress := true
	for madeProgress {
		madeProgress = false

		// 1. Scan rows
		for r := 0; r < n; r++ {
			missingCount := 0
			for c := 0; c < n; c++ {
				if eds.Matrix[r][c].IsEmpty() {
					missingCount++
				}
			}
			// If we have >= k shares present and some missing, reconstruct row
			if missingCount > 0 && (n-missingCount) >= k {
				rawShards := make([][]byte, n)
				for c := 0; c < n; c++ {
					if !eds.Matrix[r][c].IsEmpty() {
						rawShards[c] = eds.Matrix[r][c].Bytes()
					}
				}
				if err := rsCodec.Reconstruct(rawShards); err == nil {
					for c := 0; c < n; c++ {
						if eds.Matrix[r][c].IsEmpty() && len(rawShards[c]) > 0 {
							sh, err := ShareFromBytes(rawShards[c])
							if err == nil {
								eds.Matrix[r][c] = sh
								madeProgress = true
							}
						}
					}
				}
			}
		}

		// 2. Scan columns
		for c := 0; c < n; c++ {
			missingCount := 0
			for r := 0; r < n; r++ {
				if eds.Matrix[r][c].IsEmpty() {
					missingCount++
				}
			}
			if missingCount > 0 && (n-missingCount) >= k {
				rawShards := make([][]byte, n)
				for r := 0; r < n; r++ {
					if !eds.Matrix[r][c].IsEmpty() {
						rawShards[r] = eds.Matrix[r][c].Bytes()
					}
				}
				if err := rsCodec.Reconstruct(rawShards); err == nil {
					for r := 0; r < n; r++ {
						if eds.Matrix[r][c].IsEmpty() && len(rawShards[r]) > 0 {
							sh, err := ShareFromBytes(rawShards[r])
							if err == nil {
								eds.Matrix[r][c] = sh
								madeProgress = true
							}
						}
					}
				}
			}
		}
	}

	// Verify all original k x k data shares are present
	for r := 0; r < k; r++ {
		for c := 0; c < k; c++ {
			if eds.Matrix[r][c].IsEmpty() {
				return ErrReconstructionFailed
			}
		}
	}

	return nil
}
