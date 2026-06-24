package enrich

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

func TestSampleMessagesFiltersTruncatesAndKeepsOrder(t *testing.T) {
	long := strings.Repeat("界", 520)
	msgs := []db.Message{
		{Role: "system", Content: "system reminder", IsSystem: true},
		{Role: "tool", Content: "tool result should not be sampled"},
		{Role: "user", Content: "短"},
		{Role: "user", Content: "已取消"},
		{Role: "assistant", Content: "<system-reminder>ignore</system-reminder>"},
		{Role: "user", Content: "This session is being continued from a previous conversation."},
		{Role: "user", Content: "[Request interrupted by user]"},
		{Role: "user", Content: "<task-notification>noise</task-notification>"},
		{Role: "user", Content: "<command-message>noise</command-message>"},
		{Role: "user", Content: "<command-name>noise</command-name>"},
		{Role: "user", Content: "<local-command-stdout>noise</local-command-stdout>"},
		{Role: "user", Content: "Stop hook feedback: noise"},
		{Role: "user", Content: "  first useful message  "},
		{Role: "assistant", Content: long},
	}
	samples := sampleMessages(msgs)
	require.Len(t, samples, 2)
	assert.Equal(t, "first useful message", samples[0])
	assert.Equal(t, 500, len([]rune(samples[1])))
}

func TestSampleMessagesSelectsHeadMiddleTail(t *testing.T) {
	msgs := make([]db.Message, 0, 14)
	for i := 0; i < 14; i++ {
		msgs = append(msgs, db.Message{Role: "user", Content: "message number " + string(rune('a'+i))})
	}
	samples := sampleMessages(msgs)
	require.Len(t, samples, 10)
	assert.Equal(t, "message number a", samples[0])
	assert.Equal(t, "message number b", samples[1])
	assert.Equal(t, "message number c", samples[2])
	assert.Equal(t, "message number l", samples[7])
	assert.Equal(t, "message number m", samples[8])
	assert.Equal(t, "message number n", samples[9])
}
