package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/keelwave/keelwave/internal/store"
)

func TestListRunsHandler_returns200WithEnvelopedArray(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()

	require.NoError(t, ts.app.store.AgentRuns.Insert(ctx, &store.AgentRun{
		ProjectID: uuid.MustParse(ts.projID),
		AgentName: "list-handler-test",
		Status:    "running",
	}))

	var body struct {
		Data []struct {
			ID        string    `json:"id"`
			AgentName string    `json:"agent_name"`
			Status    string    `json:"status"`
			Timestamp time.Time `json:"timestamp"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/", ts.apiKey, nil, &body)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	require.NotEmpty(t, body.Data, "expected at least one run")
	assert.Equal(t, "list-handler-test", body.Data[0].AgentName)
}

func TestListRunsHandler_returns401WithoutKey(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/", "", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetRunHandler_returns404OnCrossTenantID(t *testing.T) {
	tsA := newTestServer(t)
	tsB := newTestServer(t)
	ctx := context.Background()

	runB := &store.AgentRun{ProjectID: uuid.MustParse(tsB.projID), AgentName: "x", Status: "running"}
	require.NoError(t, tsB.app.store.AgentRuns.Insert(ctx, runB))

	url := fmt.Sprintf("%s/v1/projects/%s/agent/runs/%s?at=%s", tsA.srv.URL, tsA.projID, runB.ID, runB.Timestamp.UTC().Format(time.RFC3339Nano))
	var body struct {
		Error string `json:"error"`
	}
	resp, raw := doJSON(t, http.MethodGet, url, tsA.apiKey, nil, &body)

	require.Equal(t, http.StatusNotFound, resp.StatusCode, "body=%s", raw)
	assert.Equal(t, "not found", body.Error, "must be project-scoped not-found envelope, not chi 404")
}

func TestRunHealthHandler_returns200WithRollup(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	pid := uuid.MustParse(ts.projID)

	run := &store.AgentRun{ProjectID: pid, AgentName: "health-agent", Status: "running"}
	require.NoError(t, ts.app.store.AgentRuns.Insert(ctx, run))
	require.NoError(t, ts.app.store.AgentRuns.Finish(ctx, run.ID, run.Timestamp, store.AgentRunFinish{
		Status:      "completed",
		TotalTokens: 1000,
	}))

	var body struct {
		Data []store.RunHealthRow `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/health", ts.apiKey, nil, &body)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	require.NotEmpty(t, body.Data)

	var found *store.RunHealthRow
	for i := range body.Data {
		if body.Data[i].AgentName == "health-agent" {
			found = &body.Data[i]
		}
	}
	require.NotNil(t, found, "expected health-agent row")
	assert.Equal(t, 1, found.TotalRuns)
	assert.Equal(t, 1, found.CompletedRuns)
	assert.InDelta(t, 1.0, found.CompletionRate, 0.001)
}

func TestRunHealthHandler_returns401WithoutKey(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/health", "", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRunsTimeseriesHandler_returns200Bucketed(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	pid := uuid.MustParse(ts.projID)

	run := &store.AgentRun{ProjectID: pid, AgentName: "ts", Status: "running"}
	require.NoError(t, ts.app.store.AgentRuns.Insert(ctx, run))
	require.NoError(t, ts.app.store.AgentRuns.Finish(ctx, run.ID, run.Timestamp, store.AgentRunFinish{
		Status: "completed",
	}))

	var body struct {
		Data []store.RunBucket `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/timeseries?bucket=1h", ts.apiKey, nil, &body)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	require.NotEmpty(t, body.Data)

	// gapfill returns every bucket in the window, so the run may sit in any of them
	total := 0
	for _, b := range body.Data {
		total += b.Total
	}
	assert.GreaterOrEqual(t, total, 1)
}

func TestRunsTimeseriesHandler_rejectsBadBucket(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/timeseries?bucket=99x", ts.apiKey, nil, nil)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRunsTimeseriesHandler_returns401WithoutKey(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/timeseries", "", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSummaryHandler_returnsCurrentAndPrevious(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	pid := uuid.MustParse(ts.projID)
	run := &store.AgentRun{ProjectID: pid, AgentName: "a", Status: "running"}
	require.NoError(t, ts.app.store.AgentRuns.Insert(ctx, run))
	require.NoError(t, ts.app.store.AgentRuns.Finish(ctx, run.ID, run.Timestamp, store.AgentRunFinish{
		Status: "completed", TotalTokens: 1000, DurationMs: new(150),
	}))
	var body struct {
		Data struct {
			Current  store.RunSummary `json:"current"`
			Previous store.RunSummary `json:"previous"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/summary", ts.apiKey, nil, &body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	assert.GreaterOrEqual(t, body.Data.Current.TotalRuns, 1)
}

func TestSummaryHandler_401WithoutKey(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/summary", "", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestTerminationsHandler_returns200(t *testing.T) {
	ts := newTestServer(t)
	ctx := context.Background()
	run := &store.AgentRun{ProjectID: uuid.MustParse(ts.projID), AgentName: "a", Status: "running"}
	require.NoError(t, ts.app.store.AgentRuns.Insert(ctx, run))
	require.NoError(t, ts.app.store.AgentRuns.Finish(ctx, run.ID, run.Timestamp, store.AgentRunFinish{
		Status: "completed", TerminationReason: new("clean"),
	}))
	var body struct{ Data []store.TerminationCount `json:"data"` }
	resp, raw := doJSON(t, http.MethodGet, ts.srv.URL+"/v1/projects/"+ts.projID+"/agent/runs/terminations", ts.apiKey, nil, &body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	require.NotEmpty(t, body.Data)
}
