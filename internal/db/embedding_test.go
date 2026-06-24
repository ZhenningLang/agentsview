package db

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingEncodeDecodeRoundTrip(t *testing.T) {
	vector := []float32{1, -2.5, 0.25}
	encoded, err := EncodeEmbedding(vector)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0, 128, 63}, encoded[:4], "float32 bytes should be little-endian")

	decoded, err := DecodeEmbedding(encoded, len(vector))
	require.NoError(t, err)
	assert.Equal(t, vector, decoded)
}

func TestEmbeddingDecodeRejectsBadBytes(t *testing.T) {
	_, err := DecodeEmbedding([]byte{1, 2, 3}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not divisible by 4")

	encoded, err := EncodeEmbedding([]float32{1, 2})
	require.NoError(t, err)
	_, err = DecodeEmbedding(encoded, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dimension mismatch")
}

func TestEmbeddingEncodeRejectsNonFiniteValues(t *testing.T) {
	_, err := EncodeEmbedding([]float32{float32(math.NaN())})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-finite")
}
