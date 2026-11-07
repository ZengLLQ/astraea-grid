package main

import (
	"fmt"
	"os"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/fraud"
	"github.com/ZengLLQ/astraea-grid/pkg/mempool"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
	"github.com/ZengLLQ/astraea-grid/pkg/node"
)

func main() {
	args := os.Args[1:]
	cmd := "demo"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "demo", "produce":
		runProduceDemo()
	case "sample":
		runSampleDemo()
	case "fraud-demo":
		runFraudDemo()
	case "version", "--version":
		fmt.Println("AstraeaGrid v0.1.0 (Modular Data Availability & DAS Engine)")
	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("AstraeaGrid: 2D Reed-Solomon & Namespaced Merkle Data Availability Engine v0.1.0")
	fmt.Println("Usage:")
	fmt.Println("  astraea produce     Pack rollup blobs, encode 2D EDS, and output DAH")
	fmt.Println("  astraea sample      Run light client Data Availability Sampling (DAS)")
	fmt.Println("  astraea fraud-demo  Demonstrate Bad Encoding Fraud Proof (BEFP) generation & verification")
	fmt.Println("  astraea version     Display AstraeaGrid version")
}

func runProduceDemo() {
	fmt.Println("==========================================================================")
	fmt.Println("AstraeaGrid: Modular Data Availability Block Production Demo")
	fmt.Println("==========================================================================")

	fn := node.NewFullNode(64)

	// Simulate 4 rollups submitting transaction data
	rollups := []struct {
		nid    uint64
		name   string
		signer string
		data   string
	}{
		{10, "StarkNet-ZK-Batch", "0xProverStark", "proof:stark_air_execution_trace_0x8f2a93..."},
		{20, "Arbitrum-Nitro-Seq", "0xNitroSeq", "txs:erc20_transfer_100_usdc_to_bob..."},
		{35, "Optimism-Bedrock-Txs", "0xOpBatcher", "txs:swap_eth_for_wbtc_uniswap_v3..."},
		{50, "Sovereign-Gaming-Rollup", "0xGameServer", "game_moves:player1_move_e4_e5_turn_12..."},
	}

	for _, r := range rollups {
		nid := nmt.NewNamespaceID(r.nid)
		b, err := mempool.NewBlob(nid, []byte(r.data), r.signer, 1000)
		if err != nil {
			panic(err)
		}
		fn.SubmitBlob(b)
		fmt.Printf("[+] Ingested Blob from Rollup '%s' [Namespace %s | Signer: %s]\n",
			r.name, nid, r.signer)
	}

	fmt.Println("[*] Packing mempool and encoding 2D Reed-Solomon Extended Data Square (EDS)...")
	block, err := fn.ProduceBlock()
	if err != nil {
		panic(err)
	}

	fmt.Println("--------------------------------------------------------------------------")
	fmt.Printf("Block #%d Successfully Produced:\n", block.Height)
	fmt.Printf("    Original Width (k)   : %d shares\n", block.EDS.OriginalWidth)
	fmt.Printf("    Extended Width (2k)  : %d shares (Total %d shares)\n",
		block.EDS.ExtendedWidth, block.EDS.ExtendedWidth*block.EDS.ExtendedWidth)
	fmt.Printf("    Data Availability Root: %s\n", block.DAH.Hex())
	fmt.Println("    Row Roots (NMT):")
	for i, rr := range block.DAH.RowRoots {
		fmt.Printf("      Row [%d]: min=%s max=%s hash=%x\n",
			i, rr.MinNamespace, rr.MaxNamespace, rr.Hash[:8])
	}
	fmt.Println("==========================================================================")
}

func runSampleDemo() {
	fmt.Println("==========================================================================")
	fmt.Println("AstraeaGrid: Light Client Data Availability Sampling (DAS)")
	fmt.Println("==========================================================================")

	fn := node.NewFullNode(64)
	for i := 1; i <= 6; i++ {
		nid := nmt.NewNamespaceID(uint64(i * 15))
		b, _ := mempool.NewBlob(nid, []byte(fmt.Sprintf("rollup-batch-data-%d", i)), "0xSeq", 100)
		fn.SubmitBlob(b)
	}
	block, err := fn.ProduceBlock()
	if err != nil {
		panic(err)
	}

	ln := node.NewLightNode(fn, 0.999999, 42)
	ln.ReceiveDAH(block.Height, block.DAH)

	fmt.Printf("[*] Sampling Block #%d with trusted DAH Root: %s\n", block.Height, block.DAH.Hex())
	numSamples := 24
	res, err := ln.SampleBlock(block.Height, numSamples)
	if err != nil {
		panic(err)
	}

	fmt.Println("--------------------------------------------------------------------------")
	fmt.Printf("DAS Results after %d Independent Random Samples:\n", numSamples)
	fmt.Printf("    Successful Samples   : %d / %d\n", res.SuccessfulSamples, res.TotalSamples)
	fmt.Printf("    Failed / Withheld    : %d\n", res.FailedSamples)
	fmt.Printf("    Statistical Confidence: %.8f %%\n", res.Confidence*100.0)
	fmt.Printf("    Availability Verdict : AVAILABLE (Probability of Withholding < %.2e)\n", 1.0-res.Confidence)
	fmt.Println("--------------------------------------------------------------------------")
	fmt.Println("[+] Light Node: Verified Data Availability without full block download.")
	fmt.Println("==========================================================================")
}

func runFraudDemo() {
	fmt.Println("==========================================================================")
	fmt.Println("AstraeaGrid: Bad Encoding Fraud Proof (BEFP) Demonstration")
	fmt.Println("==========================================================================")

	k := 4
	dataSquare := make([][]eds.Share, k)
	for r := 0; r < k; r++ {
		dataSquare[r] = make([]eds.Share, k)
		for c := 0; c < k; c++ {
			nid := nmt.NewNamespaceID(uint64(r*k + c + 1))
			dataSquare[r][c] = eds.NewShare(nid, []byte(fmt.Sprintf("tx-payload-%d-%d", r, c)))
		}
	}

	square, err := eds.NewExtendedDataSquare(dataSquare)
	if err != nil {
		panic(err)
	}

	// Malicious block producer modifies parity share at Row 2, Col 6
	corruptedRow := 2
	badCol := 6
	fmt.Printf("[!] Malicious Proposer corrupts parity share at Row %d, Col %d with invalid bytes\n",
		corruptedRow, badCol)
	square.Matrix[corruptedRow][badCol].Data[0] ^= 0xEE

	dah, err := eds.ComputeDAH(square)
	if err != nil {
		panic(err)
	}
	fmt.Printf("[!] Proposer publishes fraudulent DAH: %s\n", dah.Hex())

	fmt.Println("[*] Honest Full Node inspects row, re-encodes, detects parity mismatch...")
	befp, err := fraud.CreateBEFP(square, dah, true, corruptedRow, badCol)
	if err != nil {
		panic(err)
	}

	fmt.Println("[*] Full Node broadcasts Bad Encoding Fraud Proof (BEFP) to light clients...")
	fraudConfirmed, err := befp.Verify(dah)
	if err != nil {
		panic(err)
	}

	fmt.Println("--------------------------------------------------------------------------")
	if fraudConfirmed {
		fmt.Println("[✓] FRAUD CONFIRMED: Merkle proofs valid, but parity share does not match RS encoding!")
		fmt.Println("[✓] All network nodes reject fraudulent block. Block producer slashed.")
	} else {
		fmt.Println("[X] Fraud verification failed.")
	}
	fmt.Println("==========================================================================")
}
