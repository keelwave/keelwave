package alerting

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/keelwave/keelwave/internal/store"
)

// testPool is shared across the package, constructed once in TestMain (mirrors
// internal/store/store_test.go) so we don't pay pool-startup cost per test.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	addr := os.Getenv("TEST_DB_ADDR")
	if addr == "" {
		addr = "postgres://keelwave:keelwave@localhost:5432/keelwave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), addr)
	if err != nil {
		log.Fatalf("test db connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("test db ping (is `make db-up && make migrate-up` run?): %v", err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// seedProject builds an isolated project via the public store API (a throw-away
// user + org own it). Cascade DELETE on the projects FK cleans child rows —
// including agent_runs — when the test ends.
func seedProject(t *testing.T, s store.Storage) *store.Project {
	t.Helper()
	ctx := context.Background()

	u := &store.User{
		Email: fmt.Sprintf("alerting-%d@test.local", time.Now().UnixNano()),
		Name:  "alerting-test",
	}
	require.NoError(t, u.Password.Set("password-alerting-test"))
	require.NoError(t, s.Users.Create(ctx, u, nil))

	org := &store.Organization{Name: fmt.Sprintf("org-%d", time.Now().UnixNano())}
	require.NoError(t, s.Organizations.CreateWithOwner(ctx, org, u.ID))

	p := &store.Project{Name: fmt.Sprintf("proj-%d", time.Now().UnixNano())}
	require.NoError(t, s.Projects.Create(ctx, p, org.ID))

	t.Cleanup(func() {
		_ = s.Projects.Delete(ctx, p.ID)
		_, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID)
	})
	return p
}

// seedFinishedRun inserts an agent_run at now() and finishes it as completed with
// the given cost, so the continuous aggregate's real-time tail counts it.
func seedFinishedRun(t *testing.T, s store.Storage, projectID uuid.UUID, agentName string, cost float64) {
	t.Helper()
	ctx := context.Background()

	run := &store.AgentRun{
		ProjectID: projectID,
		AgentName: agentName,
		Status:    "running",
		Timestamp: time.Now(),
	}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))
	require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, store.AgentRunFinish{
		Status:       "completed",
		TotalCostUSD: &cost,
	}))
}
