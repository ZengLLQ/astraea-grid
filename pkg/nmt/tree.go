package nmt

import (
	"errors"
	"fmt"
)

var (
	ErrUnorderedLeaves   = errors.New("leaves must be pushed in non-decreasing namespace order")
	ErrEmptyTree         = errors.New("cannot calculate root of empty tree")
	ErrInvalidProofRange = errors.New("invalid range for proof")
)

// NamespacedMerkleTree represents a complete Namespaced Merkle Tree.
type NamespacedMerkleTree struct {
	leaves []NamespacedLeaf
	nodes  [][]Node // layers[0] = leaves, layers[top] = root
	root   *Node
	isBuilt bool
}

func NewNamespacedMerkleTree() *NamespacedMerkleTree {
	return &NamespacedMerkleTree{
		leaves: make([]NamespacedLeaf, 0),
	}
}

// Push adds a new leaf to the tree, enforcing strict non-decreasing namespace order.
func (t *NamespacedMerkleTree) Push(leaf NamespacedLeaf) error {
	if len(t.leaves) > 0 {
		lastNid := t.leaves[len(t.leaves)-1].NamespaceID
		if leaf.NamespaceID.Less(lastNid) {
			return fmt.Errorf("%w: pushed %s after %s", ErrUnorderedLeaves, leaf.NamespaceID, lastNid)
		}
	}
	t.leaves = append(t.leaves, leaf)
	t.isBuilt = false
	return nil
}

// Leaves returns a copy of all leaves currently in the tree.
func (t *NamespacedMerkleTree) Leaves() []NamespacedLeaf {
	res := make([]NamespacedLeaf, len(t.leaves))
	copy(res, t.leaves)
	return res
}

// Build constructs the Merkle layers and root.
func (t *NamespacedMerkleTree) Build() (*Node, error) {
	if len(t.leaves) == 0 {
		return nil, ErrEmptyTree
	}

	numLeaves := len(t.leaves)
	currentLayer := make([]Node, numLeaves)
	for i, l := range t.leaves {
		currentLayer[i] = NewLeafNode(l)
	}

	t.nodes = [][]Node{currentLayer}

	for len(currentLayer) > 1 {
		nextLayerLen := (len(currentLayer) + 1) / 2
		nextLayer := make([]Node, nextLayerLen)

		for i := 0; i < len(currentLayer); i += 2 {
			if i+1 < len(currentLayer) {
				nextLayer[i/2] = CombineNodes(currentLayer[i], currentLayer[i+1])
			} else {
				// Odd leaf promoted directly to preserve exact binary tree shape
				nextLayer[i/2] = currentLayer[i]
			}
		}

		t.nodes = append(t.nodes, nextLayer)
		currentLayer = nextLayer
	}

	t.root = &currentLayer[0]
	t.isBuilt = true
	return t.root, nil
}

// Root returns the root node of the NMT, building it if needed.
func (t *NamespacedMerkleTree) Root() (*Node, error) {
	if !t.isBuilt || t.root == nil {
		return t.Build()
	}
	return t.root, nil
}
