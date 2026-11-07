package node

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ZengLLQ/astraea-grid/pkg/das"
	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/mempool"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

var (
	ErrBlockNotFound = errors.New("block not found")
)

// Block represents a published DA block.
type Block struct {
	Height uint64
	DAH    *eds.DataAvailabilityHeader
	EDS    *eds.ExtendedDataSquare
}

// FullNode acts as a block producer and DA storage node.
type FullNode struct {
	mu      sync.RWMutex
	mempool []*mempool.Blob
	blocks  map[uint64]*Block
	height  uint64
	packer  *mempool.BlobPacker
}

func NewFullNode(shareSize int) *FullNode {
	return &FullNode{
		mempool: make([]*mempool.Blob, 0),
		blocks:  make(map[uint64]*Block),
		height:  0,
		packer:  mempool.NewBlobPacker(shareSize),
	}
}

// SubmitBlob accepts a new blob into the mempool.
func (fn *FullNode) SubmitBlob(blob *mempool.Blob) {
	fn.mu.Lock()
	defer fn.mu.Unlock()
	fn.mempool = append(fn.mempool, blob)
}

// ProduceBlock packs mempool blobs, generates 2D EDS, and computes DAH.
func (fn *FullNode) ProduceBlock() (*Block, error) {
	fn.mu.Lock()
	defer fn.mu.Unlock()

	if len(fn.mempool) == 0 {
		return nil, errors.New("cannot produce block: mempool is empty")
	}

	grid, err := fn.packer.Pack(fn.mempool)
	if err != nil {
		return nil, fmt.Errorf("failed to pack blobs: %w", err)
	}

	square, err := eds.NewExtendedDataSquare(grid)
	if err != nil {
		return nil, fmt.Errorf("failed to encode 2D EDS: %w", err)
	}

	dah, err := eds.ComputeDAH(square)
	if err != nil {
		return nil, fmt.Errorf("failed to compute DAH: %w", err)
	}

	fn.height++
	block := &Block{
		Height: fn.height,
		DAH:    dah,
		EDS:    square,
	}

	fn.blocks[fn.height] = block
	fn.mempool = make([]*mempool.Blob, 0) // Clear mempool

	return block, nil
}

// GetBlock returns the block at specified height.
func (fn *FullNode) GetBlock(height uint64) (*Block, error) {
	fn.mu.RLock()
	defer fn.mu.RUnlock()
	b, ok := fn.blocks[height]
	if !ok {
		return nil, ErrBlockNotFound
	}
	return b, nil
}

// QueryNamespaceData fetches all data for targetNid and returns an NMT proof.
func (fn *FullNode) QueryNamespaceData(height uint64, targetNid nmt.NamespaceID) (*nmt.Proof, []nmt.NamespacedLeaf, int, error) {
	fn.mu.RLock()
	defer fn.mu.RUnlock()

	b, ok := fn.blocks[height]
	if !ok {
		return nil, nil, -1, ErrBlockNotFound
	}

	// Search each row of the original square (rows 0 .. k-1) for targetNid
	for r := 0; r < b.EDS.OriginalWidth; r++ {
		tree := nmt.NewNamespacedMerkleTree()
		for c := 0; c < b.EDS.ExtendedWidth; c++ {
			sh := b.EDS.Matrix[r][c]
			_ = tree.Push(nmt.NewNamespacedLeaf(sh.NamespaceID, sh.Data))
		}

		proof, provedLeaves, err := tree.ProveNamespace(targetNid)
		if err == nil && !proof.IsAbsence {
			// Found namespace in row r!
			return proof, provedLeaves, r, nil
		}
	}

	// If not found in any row, return an absence proof from row 0
	tree0 := nmt.NewNamespacedMerkleTree()
	for c := 0; c < b.EDS.ExtendedWidth; c++ {
		sh := b.EDS.Matrix[0][c]
		_ = tree0.Push(nmt.NewNamespacedLeaf(sh.NamespaceID, sh.Data))
	}
	proof, leaves, err := tree0.ProveNamespace(targetNid)
	return proof, leaves, 0, err
}

// LightNode verifies data availability via DAS and fetches specific namespaces.
type LightNode struct {
	trustedDAHs map[uint64]*eds.DataAvailabilityHeader
	sampler     *das.LightClientSampler
	node        *FullNode
}

func NewLightNode(fn *FullNode, targetConfidence float64, seed int64) *LightNode {
	return &LightNode{
		trustedDAHs: make(map[uint64]*eds.DataAvailabilityHeader),
		sampler:     das.NewLightClientSampler(targetConfidence, seed),
		node:        fn,
	}
}

// ReceiveDAH receives a trusted block header from consensus.
func (ln *LightNode) ReceiveDAH(height uint64, dah *eds.DataAvailabilityHeader) {
	ln.trustedDAHs[height] = dah
}

// SampleBlock runs DAS for height and confirms availability.
func (ln *LightNode) SampleBlock(height uint64, numSamples int) (*das.SamplingResult, error) {
	dah, ok := ln.trustedDAHs[height]
	if !ok {
		return nil, fmt.Errorf("no trusted DAH for height %d", height)
	}

	b, err := ln.node.GetBlock(height)
	if err != nil {
		return nil, err
	}

	return ln.sampler.Sample(b.EDS, dah, numSamples)
}

// VerifyRollupNamespace queries and verifies rollup data against the trusted DAH without full block download.
func (ln *LightNode) VerifyRollupNamespace(height uint64, targetNid nmt.NamespaceID) ([]nmt.NamespacedLeaf, bool, error) {
	dah, ok := ln.trustedDAHs[height]
	if !ok {
		return nil, false, fmt.Errorf("no trusted DAH for height %d", height)
	}

	proof, leaves, rowIndex, err := ln.node.QueryNamespaceData(height, targetNid)
	if err != nil {
		return nil, false, err
	}

	if rowIndex < 0 || rowIndex >= len(dah.RowRoots) {
		return nil, false, errors.New("invalid row index in proof")
	}

	rowRoot := &dah.RowRoots[rowIndex]
	valid, err := proof.Verify(rowRoot, leaves, targetNid)
	if err != nil || !valid {
		return nil, false, fmt.Errorf("namespace proof verification failed: %w", err)
	}

	if proof.IsAbsence {
		return nil, true, nil // Verified absence (no leaves exist)
	}

	return leaves, true, nil // Verified inclusion
}
