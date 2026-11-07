package nmt

import (
	"testing"
)

func TestNamespaceOrderingInvariant(t *testing.T) {
	tree := NewNamespacedMerkleTree()

	nid1 := NewNamespaceID(10)
	nid2 := NewNamespaceID(20)
	nid3 := NewNamespaceID(15) // out of order

	if err := tree.Push(NewNamespacedLeaf(nid1, []byte("data1"))); err != nil {
		t.Fatalf("unexpected error pushing leaf 1: %v", err)
	}
	if err := tree.Push(NewNamespacedLeaf(nid2, []byte("data2"))); err != nil {
		t.Fatalf("unexpected error pushing leaf 2: %v", err)
	}
	// Pushing nid3 (15) after nid2 (20) must fail
	err := tree.Push(NewNamespacedLeaf(nid3, []byte("data3")))
	if err == nil {
		t.Fatalf("expected error pushing out-of-order namespace")
	}
}

func TestNMTInclusionProof(t *testing.T) {
	tree := NewNamespacedMerkleTree()

	nid1 := NewNamespaceID(1)
	nid2 := NewNamespaceID(2)
	nid3 := NewNamespaceID(3)
	nid4 := NewNamespaceID(4)

	leaves := []NamespacedLeaf{
		NewNamespacedLeaf(nid1, []byte("rollup1-chunk1")),
		NewNamespacedLeaf(nid2, []byte("rollup2-tx1")),
		NewNamespacedLeaf(nid2, []byte("rollup2-tx2")),
		NewNamespacedLeaf(nid2, []byte("rollup2-tx3")),
		NewNamespacedLeaf(nid3, []byte("rollup3-header")),
		NewNamespacedLeaf(nid4, []byte("rollup4-state")),
	}

	for _, l := range leaves {
		if err := tree.Push(l); err != nil {
			t.Fatalf("failed to push leaf: %v", err)
		}
	}

	root, err := tree.Root()
	if err != nil {
		t.Fatalf("failed to get root: %v", err)
	}

	// Prove namespace 2 (has 3 leaves: rollup2-tx1, tx2, tx3)
	proof, provedLeaves, err := tree.ProveNamespace(nid2)
	if err != nil {
		t.Fatalf("failed to prove namespace 2: %v", err)
	}
	if len(provedLeaves) != 3 {
		t.Fatalf("expected 3 proved leaves for namespace 2, got %d", len(provedLeaves))
	}
	if proof.IsAbsence {
		t.Fatalf("expected inclusion proof, got absence proof")
	}

	valid, err := proof.Verify(root, provedLeaves, nid2)
	if err != nil || !valid {
		t.Fatalf("proof verification failed: %v, valid=%v", err, valid)
	}

	// Tampering test 1: modify leaf data
	corruptedLeaves := make([]NamespacedLeaf, len(provedLeaves))
	copy(corruptedLeaves, provedLeaves)
	corruptedLeaves[0].Data = []byte("malicious-injected-data")
	valid, _ = proof.Verify(root, corruptedLeaves, nid2)
	if valid {
		t.Fatalf("tampered leaf data should fail verification")
	}

	// Tampering test 2: claim wrong target namespace
	valid, _ = proof.Verify(root, provedLeaves, nid3)
	if valid {
		t.Fatalf("wrong target namespace should fail verification")
	}
}

func TestNMTAbsenceProof(t *testing.T) {
	tree := NewNamespacedMerkleTree()

	nid1 := NewNamespaceID(100)
	nid2 := NewNamespaceID(200)
	nid3 := NewNamespaceID(500)

	_ = tree.Push(NewNamespacedLeaf(nid1, []byte("blockA")))
	_ = tree.Push(NewNamespacedLeaf(nid2, []byte("blockB")))
	_ = tree.Push(NewNamespacedLeaf(nid3, []byte("blockC")))

	root, err := tree.Root()
	if err != nil {
		t.Fatalf("failed to build root: %v", err)
	}

	// Target namespace 300 does not exist in tree
	absentNid := NewNamespaceID(300)
	proof, borderLeaves, err := tree.ProveNamespace(absentNid)
	if err != nil {
		t.Fatalf("failed to generate absence proof: %v", err)
	}
	if !proof.IsAbsence {
		t.Fatalf("expected absence proof, got inclusion")
	}

	valid, err := proof.Verify(root, borderLeaves, absentNid)
	if err != nil || !valid {
		t.Fatalf("absence proof verification failed: %v, valid=%v", err, valid)
	}

	// Try to verify absence of a namespace that actually exists (nid1)
	valid, _ = proof.Verify(root, borderLeaves, nid1)
	if valid {
		t.Fatalf("verifying absence of existing namespace should fail")
	}
}

func TestNMTDeterminism(t *testing.T) {
	buildTree := func() *Node {
		tree := NewNamespacedMerkleTree()
		for i := 1; i <= 8; i++ {
			_ = tree.Push(NewNamespacedLeaf(NewNamespaceID(uint64(i)), []byte{byte(i), byte(i * 2)}))
		}
		r, _ := tree.Root()
		return r
	}

	r1 := buildTree()
	r2 := buildTree()

	d1 := r1.Digest()
	d2 := r2.Digest()

	if d1 != d2 {
		t.Fatalf("NMT root digests are not deterministic")
	}
}
