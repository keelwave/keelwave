package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertPreviewURL(ts *testServer) string {
	return alertRulesURL(ts) + "/preview"
}

func TestAlertRulePreview_AggregateAndRejections(t *testing.T) {
	ts := newTestServer(t)

	var preview struct {
		Data struct {
			Value       float64 `json:"value"`
			SampleCount int     `json:"sample_count"`
			WouldBreach bool    `json:"would_breach"`
			ScopeLabel  string  `json:"scope_label"`
		} `json:"data"`
	}

	// No runs seeded: cost_burn sums to 0 over an empty window.
	resp, raw := doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":         "cost_burn",
		"comparator":     ">",
		"threshold":      5,
		"window_seconds": 300,
	}, &preview, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	assert.Equal(t, float64(0), preview.Data.Value)
	assert.Equal(t, 0, preview.Data.SampleCount)
	assert.False(t, preview.Data.WouldBreach, "0 is not > 5")

	// Preview must apply the engine's data gates, not just the comparator:
	// run_failure coalesces to a fully healthy 1 on no data, which naively
	// compares as < 2. With sample_count 0 the engine would not breach, so
	// neither may the preview.
	resp, raw = doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":         "run_failure",
		"comparator":     "<",
		"threshold":      2,
		"window_seconds": 300,
	}, &preview, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	assert.Equal(t, 0, preview.Data.SampleCount)
	assert.False(t, preview.Data.WouldBreach, "no finished runs is never a breach")

	// Event-class signals have no metric to evaluate.
	resp, _ = doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":     "loop",
		"comparator": ">",
		"threshold":  1,
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "loop is event-class")

	resp, _ = doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":     "not_a_signal",
		"comparator": ">",
		"threshold":  1,
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "unknown signal rejected by validator")

	// A draft the create endpoint would reject for comparator direction must not
	// preview as valid either.
	resp, _ = doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":         "cost_burn",
		"comparator":     "<",
		"threshold":      5,
		"window_seconds": 300,
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "cost_burn only alerts upward")

	resp, _ = doJSON(t, http.MethodPost, alertPreviewURL(ts), "", map[string]any{
		"signal":         "run_failure",
		"comparator":     ">",
		"threshold":      0.5,
		"window_seconds": 300,
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "run_failure completion rate only alerts downward")
}
