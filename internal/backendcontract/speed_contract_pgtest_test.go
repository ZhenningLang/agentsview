//go:build pgtest

package backendcontract

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/dbtest"
	postgresstore "go.kenn.io/agentsview/internal/postgres"
)

func TestSpeedStoreContractPostgres(t *testing.T) {
	pgURL := os.Getenv("TEST_PG_URL")
	if pgURL == "" {
		t.Skip("TEST_PG_URL not set; skipping PostgreSQL speed contract")
	}
	const schema = "agentsview_speed_contract"
	pg, err := postgresstore.Open(pgURL, schema, true)
	require.NoError(t, err)
	_, err = pg.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	require.NoError(t, err)
	require.NoError(t, pg.Close())

	local := dbtest.OpenTestDB(t)
	fixture := seedSpeedContractFixture(t, local)
	syncer, err := postgresstore.New(
		pgURL, schema, local, "speed-contract", true, postgresstore.SyncOptions{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, syncer.Close()) })
	require.NoError(t, syncer.EnsureSchema(context.Background()))
	_, err = syncer.Push(context.Background(), true, nil)
	require.NoError(t, err)

	store, err := postgresstore.NewStore(pgURL, schema, true)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	assertSpeedStoreContract(t, store, fixture)
}
