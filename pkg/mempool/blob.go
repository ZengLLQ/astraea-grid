package mempool

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/ZengLLQ/astraea-grid/pkg/eds"
	"github.com/ZengLLQ/astraea-grid/pkg/nmt"
)

var (
	ErrReservedNamespace = errors.New("cannot submit blob to reserved namespace")
	ErrEmptyBlob         = errors.New("blob data cannot be empty")
	ErrBlobTooLarge      = errors.New("blob exceeds maximum allowed square capacity")
)

// Blob represents a raw data blob submitted by a rollup or client to a specific namespace.
type Blob struct {
	NamespaceID nmt.NamespaceID
	Data        []byte
	Signer      string
	Fee         uint64
}

func NewBlob(nid nmt.NamespaceID, data []byte, signer string, fee uint64) (*Blob, error) {
	if nid.Equal(nmt.MinNamespace) || nid.Equal(nmt.ParityNamespace) || nid.Equal(nmt.TailPaddingNamespace) {
		return nil, ErrReservedNamespace
	}
	if len(data) == 0 {
		return nil, ErrEmptyBlob
	}
	return &Blob{
		NamespaceID: nid,
		Data:        data,
		Signer:      signer,
		Fee:         fee,
	}, nil
}

func (b *Blob) Hash() string {
	h := sha256.New()
	h.Write(b.NamespaceID[:])
	h.Write(b.Data)
	return hex.EncodeToString(h.Sum(nil))
}

// BlobPacker organizes submitted blobs into a sorted, padded k x k data square.
type BlobPacker struct {
	SharePayloadSize int
}

func NewBlobPacker(sharePayloadSize int) *BlobPacker {
	if sharePayloadSize <= 0 {
		sharePayloadSize = 128
	}
	return &BlobPacker{
		SharePayloadSize: sharePayloadSize,
	}
}

// Pack converts a batch of blobs into a k x k data square ready for 2D Reed-Solomon extension.
func (p *BlobPacker) Pack(blobs []*Blob) ([][]eds.Share, error) {
	if len(blobs) == 0 {
		return nil, errors.New("no blobs to pack")
	}

	// 1. Sort blobs by NamespaceID ascending
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].NamespaceID.Less(blobs[j].NamespaceID)
	})

	// 2. Fragment each blob into fixed-size shares
	var allShares []eds.Share
	for _, b := range blobs {
		offset := 0
		dataLen := len(b.Data)
		for offset < dataLen {
			chunkEnd := offset + p.SharePayloadSize
			if chunkEnd > dataLen {
				chunkEnd = dataLen
			}
			chunk := make([]byte, p.SharePayloadSize)
			copy(chunk, b.Data[offset:chunkEnd])

			allShares = append(allShares, eds.NewShare(b.NamespaceID, chunk))
			offset = chunkEnd
		}
	}

	totalShares := len(allShares)

	// 3. Determine square dimension k (power of 2: 2, 4, 8, 16, 32...)
	k := 2
	for (k * k) < totalShares {
		k *= 2
		if k > 64 {
			return nil, fmt.Errorf("%w: total shares %d exceeds max 64x64", ErrBlobTooLarge, totalShares)
		}
	}

	squareCapacity := k * k
	// Pad remaining shares with TailPaddingNamespace
	for len(allShares) < squareCapacity {
		padData := make([]byte, p.SharePayloadSize)
		allShares = append(allShares, eds.NewShare(nmt.TailPaddingNamespace, padData))
	}

	// 4. Arrange into k x k grid
	grid := make([][]eds.Share, k)
	for r := 0; r < k; r++ {
		grid[r] = make([]eds.Share, k)
		for c := 0; c < k; c++ {
			grid[r][c] = allShares[r*k+c]
		}
	}

	return grid, nil
}
