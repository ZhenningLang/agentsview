package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentDataVersionSkillInference(t *testing.T) {
	assert.Equal(t, 40, CurrentDataVersion())
}
