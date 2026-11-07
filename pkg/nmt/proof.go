package nmt

import (
	"errors"
	"fmt"
)

var (
	ErrProofVerificationFailed = errors.New("NMT proof verification failed")
	ErrInvalidProofStructure   = errors.New("invalid proof structure")
)

// ProofNode represents a sibling node in the Merkle co-path.
type ProofNode struct {
	Node   Node
	IsLeft bool
}

// Proof represents a cryptographic range proof from a Namespaced Merkle Tree.
type Proof struct {
	StartLeaf int
	EndLeaf   int // exclusive
	Nodes     []ProofNode
	IsAbsence bool
}

// ProveRange generates an audit proof for leaves in range [start, end).
func (t *NamespacedMerkleTree) ProveRange(start, end int) (*Proof, error) {
	if _, err := t.Root(); err != nil {
		return nil, err
	}

	numLeaves := len(t.leaves)
	if start < 0 || end > numLeaves || start >= end {
		return nil, fmt.Errorf("%w: [%d, %d) with %d leaves", ErrInvalidProofRange, start, end, numLeaves)
	}

	var proofNodes []ProofNode
	curStart := start
	curEnd := end

	// Walk from bottom layer (leaves) up to layer just below root
	for layerIdx := 0; layerIdx < len(t.nodes)-1; layerIdx++ {
		layer := t.nodes[layerIdx]

		// If curStart is odd, the left sibling at curStart-1 is outside our range
		if curStart%2 == 1 {
			proofNodes = append(proofNodes, ProofNode{
				Node:   layer[curStart-1],
				IsLeft: true,
			})
		}

		// If curEnd is odd (and not the end of the layer), the right sibling at curEnd is outside our range
		if curEnd%2 == 1 && curEnd < len(layer) {
			proofNodes = append(proofNodes, ProofNode{
				Node:   layer[curEnd],
				IsLeft: false,
			})
		}

		// Move to parent layer
		curStart /= 2
		curEnd = (curEnd + 1) / 2
	}

	return &Proof{
		StartLeaf: start,
		EndLeaf:   end,
		Nodes:     proofNodes,
		IsAbsence: false,
	}, nil
}

// ProveNamespace generates an inclusion proof or an absence proof for targetNid.
func (t *NamespacedMerkleTree) ProveNamespace(targetNid NamespaceID) (*Proof, []NamespacedLeaf, error) {
	if _, err := t.Root(); err != nil {
		return nil, nil, err
	}

	start := -1
	end := -1

	for i, l := range t.leaves {
		if l.NamespaceID.Equal(targetNid) {
			if start == -1 {
				start = i
			}
			end = i + 1
		}
	}

	if start != -1 {
		// Namespace exists -> Inclusion Proof
		proof, err := t.ProveRange(start, end)
		if err != nil {
			return nil, nil, err
		}
		return proof, t.leaves[start:end], nil
	}

	// Namespace does not exist -> Absence Proof
	// Find index where targetNid would be inserted
	insertIdx := 0
	for i, l := range t.leaves {
		if l.NamespaceID.Less(targetNid) {
			insertIdx = i + 1
		} else {
			break
		}
	}

	var proofStart, proofEnd int
	if insertIdx == 0 {
		proofStart = 0
		proofEnd = 1
	} else if insertIdx >= len(t.leaves) {
		proofStart = len(t.leaves) - 1
		proofEnd = len(t.leaves)
	} else {
		// Span the gap between insertIdx-1 and insertIdx
		proofStart = insertIdx - 1
		proofEnd = insertIdx + 1
	}

	proof, err := t.ProveRange(proofStart, proofEnd)
	if err != nil {
		return nil, nil, err
	}
	proof.IsAbsence = true

	return proof, t.leaves[proofStart:proofEnd], nil
}

// Verify verifies an NMT range proof against a trusted root Node.
func (p *Proof) Verify(trustedRoot *Node, leaves []NamespacedLeaf, targetNid NamespaceID) (bool, error) {
	if trustedRoot == nil {
		return false, errors.New("trusted root is nil")
	}

	if p.EndLeaf-p.StartLeaf != len(leaves) {
		return false, fmt.Errorf("%w: leaf count mismatch", ErrInvalidProofStructure)
	}

	// 1. Validate namespace conditions
	if !p.IsAbsence {
		// Inclusion: All leaves must have targetNid
		for _, l := range leaves {
			if !l.NamespaceID.Equal(targetNid) {
				return false, fmt.Errorf("leaf namespace %s != target %s", l.NamespaceID, targetNid)
			}
		}
	} else {
		// Absence: Must prove target falls strictly outside or within a demonstrated leaf gap
		if p.StartLeaf == 0 && len(leaves) == 1 {
			if !targetNid.Less(leaves[0].NamespaceID) {
				return false, fmt.Errorf("target %s is not strictly less than first leaf %s", targetNid, leaves[0].NamespaceID)
			}
		} else if len(leaves) == 2 {
			if !leaves[0].NamespaceID.Less(targetNid) || !targetNid.Less(leaves[1].NamespaceID) {
				return false, fmt.Errorf("target %s does not fall in gap between %s and %s",
					targetNid, leaves[0].NamespaceID, leaves[1].NamespaceID)
			}
		} else if len(leaves) == 1 {
			// Tail boundary
			if !leaves[0].NamespaceID.Less(targetNid) {
				return false, fmt.Errorf("target %s is not strictly greater than last leaf %s", targetNid, leaves[0].NamespaceID)
			}
		} else {
			return false, errors.New("invalid leaf count for absence proof")
		}
	}

	// 2. Build initial layer from leaves
	currentLayer := make([]Node, len(leaves))
	for i, l := range leaves {
		currentLayer[i] = NewLeafNode(l)
	}

	// Reconstruct root using proofNodes
	nodeIdx := 0
	curStart := p.StartLeaf
	curEnd := p.EndLeaf

	for len(currentLayer) > 1 || nodeIdx < len(p.Nodes) {
		// If left sibling exists at this layer
		if curStart%2 == 1 {
			if nodeIdx >= len(p.Nodes) || !p.Nodes[nodeIdx].IsLeft {
				return false, errors.New("expected left proof node missing")
			}
			leftNode := p.Nodes[nodeIdx].Node
			nodeIdx++
			currentLayer = append([]Node{leftNode}, currentLayer...)
			curStart--
		}

		// If right sibling exists at this layer
		if curEnd%2 == 1 {
			if nodeIdx < len(p.Nodes) && !p.Nodes[nodeIdx].IsLeft {
				rightNode := p.Nodes[nodeIdx].Node
				nodeIdx++
				currentLayer = append(currentLayer, rightNode)
				curEnd++
			}
		}

		// Reduce currentLayer by pairing adjacent nodes
		nextLayer := make([]Node, (len(currentLayer)+1)/2)
		for i := 0; i < len(currentLayer); i += 2 {
			if i+1 < len(currentLayer) {
				nextLayer[i/2] = CombineNodes(currentLayer[i], currentLayer[i+1])
			} else {
				nextLayer[i/2] = currentLayer[i]
			}
		}

		currentLayer = nextLayer
		curStart /= 2
		curEnd = (curEnd + 1) / 2
	}

	if len(currentLayer) != 1 {
		return false, errors.New("failed to reduce layer to single root")
	}

	computedRoot := currentLayer[0]
	if computedRoot.Digest() != trustedRoot.Digest() {
		return false, fmt.Errorf("%w: computed root %s != trusted %s",
			ErrProofVerificationFailed, computedRoot, trustedRoot)
	}

	return true, nil
}
