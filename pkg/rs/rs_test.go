package rs

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestGF256Arithmetic(t *testing.T) {
	gf := DefaultGF256

	// Test addition is XOR
	if gf.Add(0x57, 0x83) != (0x57 ^ 0x83) {
		t.Fatalf("GF256 addition mismatch")
	}

	// Test multiplication by 1 and 0
	for a := 1; a < 256; a++ {
		if gf.Mul(byte(a), 0) != 0 {
			t.Fatalf("mul by zero failed for %d", a)
		}
		if gf.Mul(byte(a), 1) != byte(a) {
			t.Fatalf("mul by one failed for %d", a)
		}

		inv, err := gf.Inv(byte(a))
		if err != nil {
			t.Fatalf("unexpected inv error for %d: %v", a, err)
		}
		prod := gf.Mul(byte(a), inv)
		if prod != 1 {
			t.Fatalf("a * inv(a) != 1 for a=%d, inv=%d, prod=%d", a, inv, prod)
		}
	}
}

func TestMatrixInversion(t *testing.T) {
	gf := DefaultGF256
	n := 4
	m := CauchyMatrix(n, n)

	inv, err := m.Invert()
	if err != nil {
		t.Fatalf("cauchy matrix failed to invert: %v", err)
	}

	// Check M * inv(M) == Identity
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum byte
			for k := 0; k < n; k++ {
				sum = gf.Add(sum, gf.Mul(m.Data[i][k], inv.Data[k][j]))
			}
			expected := byte(0)
			if i == j {
				expected = 1
			}
			if sum != expected {
				t.Fatalf("identity mismatch at (%d, %d): got %d, expected %d", i, j, sum, expected)
			}
		}
	}
}

func TestReedSolomonReconstruction(t *testing.T) {
	k := 4
	m := 4
	rs, err := NewReedSolomon(k, m)
	if err != nil {
		t.Fatalf("failed to create ReedSolomon: %v", err)
	}

	shardSize := 64
	originalShards := make([][]byte, rs.TotalShards)
	for i := 0; i < k; i++ {
		originalShards[i] = make([]byte, shardSize)
		rand.Read(originalShards[i])
	}

	if err := rs.Encode(originalShards); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	valid, err := rs.Verify(originalShards)
	if err != nil || !valid {
		t.Fatalf("verify failed on freshly encoded shards: %v, valid=%v", err, valid)
	}

	// Test 1: Drop 2 data shards and 2 parity shards
	corrupted := make([][]byte, rs.TotalShards)
	for i := 0; i < rs.TotalShards; i++ {
		corrupted[i] = make([]byte, shardSize)
		copy(corrupted[i], originalShards[i])
	}
	corrupted[0] = nil
	corrupted[2] = nil
	corrupted[4] = nil
	corrupted[6] = nil

	if err := rs.Reconstruct(corrupted); err != nil {
		t.Fatalf("reconstruction failed: %v", err)
	}

	for i := 0; i < rs.TotalShards; i++ {
		if !bytes.Equal(corrupted[i], originalShards[i]) {
			t.Fatalf("shard %d mismatch after reconstruction", i)
		}
	}

	// Test 2: Drop all k data shards (keep only m parity shards)
	onlyParity := make([][]byte, rs.TotalShards)
	for i := 0; i < rs.TotalShards; i++ {
		if i >= k {
			onlyParity[i] = make([]byte, shardSize)
			copy(onlyParity[i], originalShards[i])
		}
	}

	if err := rs.Reconstruct(onlyParity); err != nil {
		t.Fatalf("reconstruction from parity only failed: %v", err)
	}

	for i := 0; i < k; i++ {
		if !bytes.Equal(onlyParity[i], originalShards[i]) {
			t.Fatalf("data shard %d mismatch when reconstructed purely from parity", i)
		}
	}

	// Test 3: Drop 5 shards (exceeding tolerance m = 4)
	tooFew := make([][]byte, rs.TotalShards)
	for i := 0; i < 3; i++ {
		tooFew[i] = make([]byte, shardSize)
		copy(tooFew[i], originalShards[i])
	}
	err = rs.Reconstruct(tooFew)
	if err == nil {
		t.Fatalf("expected error when reconstructing with insufficient shards")
	}
}

func BenchmarkRSEncode16x16(b *testing.B) {
	k := 16
	rs, err := NewReedSolomon(k, k)
	if err != nil {
		b.Fatal(err)
	}

	shardSize := 512
	shards := make([][]byte, rs.TotalShards)
	for i := 0; i < k; i++ {
		shards[i] = make([]byte, shardSize)
		rand.Read(shards[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rs.Encode(shards)
	}
}

