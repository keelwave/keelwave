package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/alerting"
	"github.com/keelwave/keelwave/internal/auth"
	"github.com/keelwave/keelwave/internal/batch"
	"github.com/keelwave/keelwave/internal/mailer"
	"github.com/keelwave/keelwave/internal/store"
)

type noopMailer struct{}

func (noopMailer) Send(_ string, _ string, _ any) error { return nil }

var _ mailer.Client = noopMailer{}

const testCookieName = "kw_session"

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
		log.Fatalf("test db ping (run `make db-up && make migrate-up`): %v", err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

type testServer struct {
	srv    *httptest.Server
	projID string
	orgID  string
	apiKey string
	app    *application
	cookie *http.Cookie // session cookie for userAuthMiddleware-protected routes
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx := context.Background()

	s := store.NewStorage(testPool)

	u := &store.User{Email: fmt.Sprintf("test-%d@dev.local", time.Now().UnixNano()), Name: "test", IsVerified: true}
	require.NoError(t, u.Password.Set("testpass"))
	require.NoError(t, s.Users.Create(ctx, u, nil))

	org := &store.Organization{Name: fmt.Sprintf("org-%d", time.Now().UnixNano())}
	require.NoError(t, s.Organizations.CreateWithOwner(ctx, org, u.ID))

	proj := &store.Project{Name: fmt.Sprintf("test-%d", time.Now().UnixNano())}
	require.NoError(t, s.Projects.Create(ctx, proj, org.ID))

	apiPlaintext, apiHash, err := auth.Generate()
	require.NoError(t, err)
	key := &store.APIKey{ProjectID: proj.ID, KeyHash: apiHash, Name: "test"}
	require.NoError(t, s.APIKeys.Create(ctx, key))

	sessPlaintext, sessHash, err := auth.GenerateSession()
	require.NoError(t, err)
	sess := &store.Session{
		UserID:    u.ID,
		TokenHash: sessHash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, s.Sessions.Create(ctx, sess))

	app := &application{
		config: config{
			env: "test",
			rateLimit: rateLimitConfig{
				ingestIPPerMinute:  1_000_000,
				ingestKeyPerMinute: 1_000_000,
				ingestWindow:       time.Minute,
			},
			auth: authConfig{
				cookieName:   testCookieName,
				sessionTTL:   24 * time.Hour,
				dashboardURL: "http://localhost:3000",
			},
		},
		pool:      testPool,
		store:     s,
		batchers:  batch.NewBatchers(testPool, batch.Config{}, zap.NewNop().Sugar()),
		mailer:    noopMailer{},
		logger:    zap.NewNop().Sugar(),
		evaluator: alerting.NewEvaluator(testPool, s, zap.NewNop().Sugar()),
	}

	srv := httptest.NewServer(app.mount())
	t.Cleanup(func() {
		srv.Close()
		_ = s.Projects.Delete(ctx, proj.ID)
	})

	return &testServer{
		srv:    srv,
		projID: proj.ID.String(),
		orgID:  org.ID.String(),
		apiKey: apiPlaintext,
		app:    app,
		cookie: &http.Cookie{Name: testCookieName, Value: sessPlaintext},
	}
}

// doJSON issues a request and unmarshals the response body.
// Pass cookies (e.g. ts.cookie for session auth) as optional trailing args.
func doJSON(t *testing.T, method, url, key string, body any, out any, cookies ...*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = nopReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	if out != nil && len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, out), "body=%s", raw)
	}
	return resp, raw
}

type nopReader []byte

func (b nopReader) Read(p []byte) (int, error) {
	n := copy(p, b)
	if n < len(b) {
		return n, nil
	}
	return n, io.EOF
}
