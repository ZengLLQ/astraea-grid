package nmt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	LeafPrefix = 0x00
	NodePrefix = 0x01
)

// NamespacedLeaf represents a leaf in the Namespaced Merkle Tree.
type NamespacedLeaf struct {
	NamespaceID NamespaceID
	Data        []byte
}

func NewNamespacedLeaf(nid NamespaceID, data []byte) NamespacedLeaf {
	return NamespacedLeaf{
		NamespaceID: nid,
		Data:        data,
	}
}

// Node represents an internal or leaf node in the Namespaced Merkle Tree.
type Node struct {
	MinNamespace NamespaceID
	MaxNamespace NamespaceID
	Hash         [32]byte
}

// NewLeafNode constructs an NMT node for a single leaf.
func NewLeafNode(leaf NamespacedLeaf) Node {
	h := sha256.New()
	h.Write([]byte{LeafPrefix})
	h.Write(leaf.NamespaceID[:])
	h.Write(leaf.Data)
	var hash [32]byte
	copy(hash[:], h.Sum(nil))

	return Node{
		MinNamespace: leaf.NamespaceID,
		MaxNamespace: leaf.NamespaceID,
		Hash:         hash,
	}
}

// CombineNodes combines left and right child nodes into a parent NMT node.
func CombineNodes(left, right Node) Node {
	minNid := left.MinNamespace
	if right.MinNamespace.Less(minNid) {
		minNid = right.MinNamespace
	}

	maxNid := left.MaxNamespace
	if maxNid.Less(right.MaxNamespace) {
		maxNid = right.MaxNamespace
	}

	h := sha256.New()
	h.Write([]byte{NodePrefix})
	h.Write(minNid[:])
	h.Write(maxNid[:])
	h.Write(left.Hash[:])
	h.Write(right.Hash[:])
	var hash [32]byte
	copy(hash[:], h.Sum(nil))

	return Node{
		MinNamespace: minNid,
		MaxNamespace: maxNid,
		Hash:         hash,
	}
}

// Digest returns the full serialized representation of the NMT node:
// [MinNamespace (8 bytes) || MaxNamespace (8 bytes) || Hash (32 bytes)] = 48 bytes.
func (n Node) Digest() [48]byte {
	var d [48]byte
	copy(d[0:8], n.MinNamespace[:])
	copy(d[8:16], n.MaxNamespace[:])
	copy(d[16:48], n.Hash[:])
	return d
}

func (n Node) String() string {
	return fmt.Sprintf("Node[min=%s, max=%s, hash=%s]",
		n.MinNamespace.Hex()[:6],
		n.MaxNamespace.Hex()[:6],
		hex.EncodeToString(n.Hash[:8]))
}
