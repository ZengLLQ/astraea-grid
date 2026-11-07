package das

import (
	"testing"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

func makeTestEDS(k int) (*eds.ExtendedDataSquare, *eds.DataAvailabilityHeader) {
	dataSquare := make([][]eds.Share, k)
	for r := 0; r < k; r++ {
		dataSquare[r] = make([]eds.Share, k)
		for c := 0; c < k; c++ {
			nid := nmt.NewNamespaceID(uint64(r*k + c + 1))
			data := []byte{byte(r), byte(c), 0xAA, 0xBB}
			dataSquare[r][c] = eds.NewShare(nid, data)
		}
	}
	square, err := eds.NewExtendedDataSquare(dataSquare)
	if err != nil {
		panic(err)
	}
	dah, err := eds.ComputeDAH(square)
	if err != nil {
		panic(err)
	}
	return square, dah
}

func TestDASConfidenceCalculation(t *testing.T) {
	c1 := CalculateConfidence(1)
	if (c1 - 0.5) > 1e-6 {
		t.Fatalf("expected 0.5 confidence for 1 sample, got %f", c1)
	}

	c10 := CalculateConfidence(10)
	if c10 < 0.999 {
		t.Fatalf("expected >0.999 confidence for 10 samples, got %f", c10)
	}

	c20 := CalculateConfidence(20)
	if c20 < 0.999999 {
		t.Fatalf("expected >0.999999 confidence for 20 samples, got %f", c20)
	}

	c30 := CalculateConfidence(30)
	if c30 < 0.9999999 {
		t.Fatalf("expected >0.9999999 confidence for 30 samples, got %f", c30)
	}
}

func TestDASSampleVerification(t *testing.T) {
	square, dah := makeTestEDS(4)

	coord := Coordinate{Row: 2, Col: 3}
	sample, err := CreateSample(square, coord)
	if err != nil {
		t.Fatalf("failed to create sample: %v", err)
	}

	valid, err := sample.Verify(dah)
	if err != nil || !valid {
		t.Fatalf("sample verification failed: %v, valid=%v", err, valid)
	}

	// Corrupt share payload
	sample.Share.Data[0] ^= 0xFF
	valid, _ = sample.Verify(dah)
	if valid {
		t.Fatalf("corrupted sample should fail verification")
	}
}

func TestLightClientSamplingFullSquare(t *testing.T) {
	square, dah := makeTestEDS(4)

	sampler := NewLightClientSampler(0.9999, 42)
	res, err := sampler.Sample(square, dah, 16)
	if err != nil {
		t.Fatalf("sampling failed: %v", err)
	}

	if res.FailedSamples > 0 || !res.IsAvailable {
		t.Fatalf("expected fully available block, got: %s", res)
	}
	if res.Confidence < 0.9999 {
		t.Fatalf("confidence below threshold: %f", res.Confidence)
	}
}

func TestLightClientSamplingDataWithholding(t *testing.T) {
	square, dah := makeTestEDS(4)

	// Adversary withholds 50% of the entire square
	for r := 0; r < square.ExtendedWidth; r++ {
		for c := 0; c < square.OriginalWidth; c++ {
			square.Matrix[r][c] = eds.Share{} // withheld
		}
	}

	sampler := NewLightClientSampler(0.9999, 999)
	res, err := sampler.Sample(square, dah, 20)
	if err != nil {
		t.Fatalf("sampling error: %v", err)
	}

	// With 50% withheld, 20 samples will almost certainly catch missing shares
	if res.FailedSamples == 0 || res.IsAvailable {
		t.Fatalf("expected data withholding to be detected, got: %s", res)
	}
}
