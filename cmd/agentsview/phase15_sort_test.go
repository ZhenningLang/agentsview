package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase15SessionListSortFlags(t *testing.T) {
	cmd := newSessionListCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.ParseFlags([]string{"--sort", "messages:desc,started:asc", "--reverse"}))
	flag := cmd.Flags().Lookup("sort")
	require.NotNil(t, flag)
	assert.Equal(t, "messages:desc,started:asc", flag.Value.String())
	assert.NotNil(t, cmd.Flags().Lookup("reverse"))

	_, err := cmd.Flags().GetBool("reverse")
	require.NoError(t, err)
}

func TestPhase15SessionListSortRejectsInvalidSpec(t *testing.T) {
	_, err := executeCommand(newRootCommand(), "session", "list", "--sort", "messages:nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sort")
}

func TestPhase15SessionListSortForwardedToServer(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"explicit multi", []string{"--sort", "messages:desc,started:asc"}, "order_by=messages%3Adesc%2Cstarted%3Aasc"},
		{"reverse default", []string{"--reverse"}, "order_by=recent%3Aasc"},
		{"reverse bare", []string{"--sort", "messages", "--reverse"}, "order_by=messages%3Adesc"},
		{"explicit ignores reverse", []string{"--sort", "messages:asc", "--reverse"}, "order_by=messages%3Aasc"},
		{"mixed fallback", []string{"--sort", "messages:desc,started", "--reverse"}, "order_by=messages%3Adesc%2Cstarted%3Adesc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rawQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"sessions":[],"total":0}`))
			}))
			t.Cleanup(srv.Close)

			args := []string{"session", "list", "--server", srv.URL, "--json"}
			args = append(args, tt.args...)
			_, err := executeCommand(newRootCommand(), args...)
			require.NoError(t, err)
			assert.Contains(t, rawQuery, tt.want)
		})
	}
}
