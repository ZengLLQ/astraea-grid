package das

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
)

// SamplingResult contains statistical metrics from a Data Availability Sampling round.
type SamplingResult struct {
	TotalSamples      int
	SuccessfulSamples int
	FailedSamples     int
	Confidence        float64
	IsAvailable       bool
	SampleCoords      []Coordinate
}

// LightClientSampler executes DAS rounds to verify data availability without downloading full blocks.
type LightClientSampler struct {
	TargetConfidence float64
	RNG              *rand.Rand
}

func NewLightClientSampler(targetConfidence float64, seed int64) *LightClientSampler {
	if targetConfidence <= 0.0 || targetConfidence >= 1.0 {
		targetConfidence = 0.999999 // 99.9999% default (20 samples)
	}
	var rng *rand.Rand
	if seed != 0 {
		rng = rand.New(rand.NewSource(seed))
	} else {
		rng = rand.New(rand.NewSource(1337))
	}
	return &LightClientSampler{
		TargetConfidence: targetConfidence,
		RNG:              rng,
	}
}

// Sample performs independent random sampling across the 2D EDS.
func (s *LightClientSampler) Sample(
	square *eds.ExtendedDataSquare,
	dah *eds.DataAvailabilityHeader,
	numSamples int,
) (*SamplingResult, error) {
	if square == nil || dah == nil {
		return nil, errors.New("square and DAH must be non-nil")
	}

	n := square.ExtendedWidth
	sampledSet := make(map[Coordinate]bool)
	var coords []Coordinate

	// Pick unique random coordinates
	for len(coords) < numSamples && len(coords) < (n*n) {
		r := s.RNG.Intn(n)
		c := s.RNG.Intn(n)
		coord := Coordinate{Row: r, Col: c}
		if !sampledSet[coord] {
			sampledSet[coord] = true
			coords = append(coords, coord)
		}
	}

	successful := 0
	failed := 0

	for _, coord := range coords {
		// If share is missing in the square, sample fails immediately (data withholding detected)
		if square.Matrix[coord.Row][coord.Col].IsEmpty() {
			failed++
			continue
		}

		sample, err := CreateSample(square, coord)
		if err != nil {
			failed++
			continue
		}

		valid, err := sample.Verify(dah)
		if err != nil || !valid {
			failed++
		} else {
			successful++
		}
	}

	conf := CalculateConfidence(successful)
	isAvailable := (failed == 0) && (conf >= s.TargetConfidence)

	return &SamplingResult{
		TotalSamples:      len(coords),
		SuccessfulSamples: successful,
		FailedSamples:     failed,
		Confidence:        conf,
		IsAvailable:       isAvailable,
		SampleCoords:      coords,
	}, nil
}

func (res *SamplingResult) String() string {
	return fmt.Sprintf("DAS Result: %d/%d samples verified | Confidence: %.6f%% | Available: %v",
		res.SuccessfulSamples, res.TotalSamples, res.Confidence*100.0, res.IsAvailable)
}
