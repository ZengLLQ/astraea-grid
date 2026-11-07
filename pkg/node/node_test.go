package node

import (
	"bytes"
	"testing"

	"github.com/ZengLLQ/astraea-grid/pkg/mempool"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

func TestFullNodeBlockProductionAndLightNodeDAS(t *testing.T) {
	fn := NewFullNode(64)

	nid1 := nmt.NewNamespaceID(10)
	nid2 := nmt.NewNamespaceID(25)
	nid3 := nmt.NewNamespaceID(40)

	b1, _ := mempool.NewBlob(nid1, []byte("defi-rollup-state-update-batch-001"), "0xDefiSeq", 500)
	b2, _ := mempool.NewBlob(nid2, []byte("nft-game-zk-proof-batch-002"), "0xGameSeq", 300)
	b3, _ := mempool.NewBlob(nid3, []byte("oracle-feed-prices-batch-003"), "0xOracleSeq", 200)

	fn.SubmitBlob(b1)
	fn.SubmitBlob(b2)
	fn.SubmitBlob(b3)

	block, err := fn.ProduceBlock()
	if err != nil {
		t.Fatalf("failed to produce block: %v", err)
	}

	if block.Height != 1 {
		t.Fatalf("expected height 1, got %d", block.Height)
	}

	// Light client interacts with FullNode
	ln := NewLightNode(fn, 0.9999, 1337)
	ln.ReceiveDAH(block.Height, block.DAH)

	// 1. Run DAS
	samplingRes, err := ln.SampleBlock(block.Height, 16)
	if err != nil {
		t.Fatalf("DAS sampling error: %v", err)
	}
	if !samplingRes.IsAvailable || samplingRes.FailedSamples > 0 {
		t.Fatalf("DAS sampling failed: %s", samplingRes)
	}

	// 2. Query and verify Rollup 25 (nft-game)
	leaves, valid, err := ln.VerifyRollupNamespace(block.Height, nid2)
	if err != nil || !valid {
		t.Fatalf("failed to verify rollup namespace 25: %v, valid=%v", err, valid)
	}
	if len(leaves) == 0 {
		t.Fatalf("expected leaves for namespace 25, got 0")
	}
	if !bytes.Contains(leaves[0].Data, []byte("nft-game-zk-proof-batch-002")) {
		t.Fatalf("unexpected leaf content: %s", string(leaves[0].Data))
	}

	// 3. Query non-existent namespace 99 (absence proof)
	absentNid := nmt.NewNamespaceID(99)
	absentLeaves, validAbsence, err := ln.VerifyRollupNamespace(block.Height, absentNid)
	if err != nil || !validAbsence {
		t.Fatalf("failed to verify absence of namespace 99: %v, valid=%v", err, validAbsence)
	}
	if len(absentLeaves) != 0 {
		t.Fatalf("expected 0 leaves for absent namespace, got %d", len(absentLeaves))
	}
}
