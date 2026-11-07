package rs

import (
	"errors"
	"fmt"
)

var (
	ErrZeroDivision     = errors.New("division by zero in GF(256)")
	ErrSingularMatrix   = errors.New("singular matrix over GF(256)")
	ErrInsufficientShards = errors.New("insufficient shards available for reconstruction")
)

// GF256 represents Galois Field GF(2^8) arithmetic.
// Uses irreducible polynomial: x^8 + x^4 + x^3 + x^2 + 1 (0x11D, decimal 285).
type GF256 struct {
	expTable [512]byte
	logTable [256]byte
}

var DefaultGF256 = newGF256()

func newGF256() *GF256 {
	gf := &GF256{}
	var x byte = 1
	for i := 0; i < 255; i++ {
		gf.expTable[i] = x
		gf.expTable[i+255] = x
		gf.logTable[x] = byte(i)

		// Multiply by alpha = 0x02
		if (x & 0x80) != 0 {
			x = (x << 1) ^ 0x1D
		} else {
			x <<= 1
		}
	}
	gf.logTable[0] = 0 // log(0) is mathematically undefined, sentinel
	return gf
}

// Add returns a + b in GF(256), which is bitwise XOR.
func (gf *GF256) Add(a, b byte) byte {
	return a ^ b
}

// Sub returns a - b in GF(256), identical to addition.
func (gf *GF256) Sub(a, b byte) byte {
	return a ^ b
}

// Mul returns a * b in GF(256).
func (gf *GF256) Mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	logA := int(gf.logTable[a])
	logB := int(gf.logTable[b])
	return gf.expTable[logA+logB]
}

// Div returns a / b in GF(256). Returns error if b == 0.
func (gf *GF256) Div(a, b byte) (byte, error) {
	if b == 0 {
		return 0, ErrZeroDivision
	}
	if a == 0 {
		return 0, nil
	}
	logA := int(gf.logTable[a])
	logB := int(gf.logTable[b])
	return gf.expTable[(logA-logB+255)%255], nil
}

// Inv returns the multiplicative inverse 1 / a in GF(256).
func (gf *GF256) Inv(a byte) (byte, error) {
	if a == 0 {
		return 0, ErrZeroDivision
	}
	return gf.expTable[255-int(gf.logTable[a])], nil
}

// MulSlice multiplies all bytes in slice by scalar in-place.
func (gf *GF256) MulSlice(scalar byte, slice []byte) {
	if scalar == 0 {
		for i := range slice {
			slice[i] = 0
		}
		return
	}
	if scalar == 1 {
		return
	}
	logS := int(gf.logTable[scalar])
	for i := range slice {
		if slice[i] != 0 {
			slice[i] = gf.expTable[logS+int(gf.logTable[slice[i]])]
		}
	}
}

// AddMulSlice computes dst[i] ^= scalar * src[i] for all i.
func (gf *GF256) AddMulSlice(scalar byte, src, dst []byte) {
	if scalar == 0 {
		return
	}
	logS := int(gf.logTable[scalar])
	for i := range dst {
		if src[i] != 0 {
			dst[i] ^= gf.expTable[logS+int(gf.logTable[src[i]])]
		}
	}
}

// Matrix represents an m x n matrix over GF(256).
type Matrix struct {
	Rows int
	Cols int
	Data [][]byte
}

func NewMatrix(rows, cols int) *Matrix {
	data := make([][]byte, rows)
	for i := range data {
		data[i] = make([]byte, cols)
	}
	return &Matrix{
		Rows: rows,
		Cols: cols,
		Data: data,
	}
}

// IdentityMatrix returns an n x n identity matrix over GF(256).
func IdentityMatrix(n int) *Matrix {
	m := NewMatrix(n, n)
	for i := 0; i < n; i++ {
		m.Data[i][i] = 1
	}
	return m
}

// CauchyMatrix builds a Cauchy matrix P where P[i][j] = 1 / (x[i] ^ y[j]).
// Sets X and Y must be disjoint subsets of GF(256).
func CauchyMatrix(rows, cols int) *Matrix {
	m := NewMatrix(rows, cols)
	// Let X = [0 .. rows-1] and Y = [rows .. rows+cols-1]
	for i := 0; i < rows; i++ {
		xi := byte(i)
		for j := 0; j < cols; j++ {
			yj := byte(rows + j)
			diff := xi ^ yj
			inv, err := DefaultGF256.Inv(diff)
			if err != nil {
				panic(fmt.Sprintf("Cauchy points not distinct: %d and %d", xi, yj))
			}
			m.Data[i][j] = inv
		}
	}
	return m
}

// Invert inverts an n x n matrix using Gaussian elimination over GF(256).
func (m *Matrix) Invert() (*Matrix, error) {
	if m.Rows != m.Cols {
		return nil, fmt.Errorf("matrix must be square, got %dx%d", m.Rows, m.Cols)
	}
	n := m.Rows
	gf := DefaultGF256

	// Augment matrix with Identity: [M | I]
	aug := NewMatrix(n, 2*n)
	for i := 0; i < n; i++ {
		copy(aug.Data[i][:n], m.Data[i])
		aug.Data[i][n+i] = 1
	}

	// Forward elimination with partial pivoting
	for col := 0; col < n; col++ {
		// Find non-zero pivot
		pivotRow := -1
		for row := col; row < n; row++ {
			if aug.Data[row][col] != 0 {
				pivotRow = row
				break
			}
		}
		if pivotRow == -1 {
			return nil, ErrSingularMatrix
		}

		// Swap rows if necessary
		if pivotRow != col {
			aug.Data[col], aug.Data[pivotRow] = aug.Data[pivotRow], aug.Data[col]
		}

		// Scale pivot row to 1
		pivotVal := aug.Data[col][col]
		invPivot, err := gf.Inv(pivotVal)
		if err != nil {
			return nil, err
		}
		for j := 0; j < 2*n; j++ {
			aug.Data[col][j] = gf.Mul(aug.Data[col][j], invPivot)
		}

		// Eliminate column in other rows
		for row := 0; row < n; row++ {
			if row != col && aug.Data[row][col] != 0 {
				factor := aug.Data[row][col]
				for j := 0; j < 2*n; j++ {
					aug.Data[row][j] = gf.Sub(aug.Data[row][j], gf.Mul(factor, aug.Data[col][j]))
				}
			}
		}
	}

	// Extract right half as inverse
	inv := NewMatrix(n, n)
	for i := 0; i < n; i++ {
		copy(inv.Data[i], aug.Data[i][n:])
	}
	return inv, nil
}
