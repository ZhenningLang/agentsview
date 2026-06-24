package db

import (
	"encoding/binary"
	"fmt"
	"math"
)

type EmbeddingFilter struct {
	Project string
	Limit   int
}

type SessionEmbedding struct {
	SessionID      string
	Project        string
	Agent          string
	Name           string
	SessionEndedAt string
	Vector         []float32
}

func EncodeEmbedding(vector []float32) ([]byte, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("embedding: empty vector")
	}
	out := make([]byte, len(vector)*4)
	for i, v := range vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("embedding: non-finite value at index %d", i)
		}
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out, nil
}

func DecodeEmbedding(data []byte, dim int) ([]float32, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("embedding: invalid dimension %d", dim)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("embedding: empty bytes")
	}
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("embedding: byte length %d is not divisible by 4", len(data))
	}
	if len(data)/4 != dim {
		return nil, fmt.Errorf("embedding: dimension mismatch bytes=%d dim=%d", len(data)/4, dim)
	}
	out := make([]float32, dim)
	for i := range out {
		v := math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("embedding: non-finite value at index %d", i)
		}
		out[i] = v
	}
	return out, nil
}
