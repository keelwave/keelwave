package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Aggregate comparator direction is validated per signal: run_failure is
// lower-is-bad (completion rate), so it accepts only "<"/"<="; a ">" is a 400.
func TestAlertRuleHandler_rejectsWrongComparatorDirection(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":       "completion up",
		"class":      "aggregate",
		"signal":     "run_failure",
		"comparator": ">",
		"threshold":  0.9,
		"severity":   "page",
		"channel":    "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"lower-is-bad run_failure with '>' must be rejected, body=%s", raw)

	// cost_burn is higher-is-bad, so a "<" is rejected too.
	resp, raw = doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":       "cost down",
		"class":      "aggregate",
		"signal":     "cost_burn",
		"comparator": "<",
		"threshold":  5,
		"severity":   "page",
		"channel":    "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"higher-is-bad cost_burn with '<' must be rejected, body=%s", raw)
}

// An aggregate rule with the comparator OMITTED defaults to the signal's
// natural direction ("<" for run_failure), not ">".
func TestAlertRuleHandler_aggregateComparatorDefaultsPerSignal(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	var created struct {
		Data struct {
			ID         string `json:"id"`
			Comparator string `json:"comparator"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":      "completion default",
		"class":     "aggregate",
		"signal":    "run_failure",
		"threshold": 0.9,
		"severity":  "page",
		"channel":   "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, &created, ts.cookie)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "omitted comparator should be accepted, body=%s", raw)
	assert.Equal(t, "<", created.Data.Comparator,
		"omitted run_failure comparator defaults to its natural direction '<'")
}

// An email rule must carry a recipient "to" in channel_config.
func TestAlertRuleHandler_emailRequiresRecipient(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	// No channel_config at all → 400.
	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "no recipient",
		"class":    "event",
		"signal":   "loop",
		"severity": "page",
		"channel":  "email",
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"email rule without a 'to' must be rejected, body=%s", raw)

	// Non-email-looking "to" → 400.
	resp, raw = doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "bad recipient",
		"class":    "event",
		"signal":   "loop",
		"severity": "page",
		"channel":  "email",
		"channel_config": map[string]any{
			"to": "notanemail",
		},
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"email rule with a non-email 'to' must be rejected, body=%s", raw)

	// Valid recipient → 201.
	resp, raw = doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "good recipient",
		"class":    "event",
		"signal":   "loop",
		"severity": "page",
		"channel":  "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"email rule with a valid 'to' must be accepted, body=%s", raw)
}

// A PATCH that changes class or signal must be rejected (both are immutable in
// the store; silently ignoring the change misvalidates the body).
func TestAlertRuleHandler_rejectsClassSignalChangeOnUpdate(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":      "cost burn",
		"class":     "aggregate",
		"signal":    "cost_burn",
		"threshold": 5,
		"severity":  "warn",
		"channel":   "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, &created, ts.cookie)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body=%s", raw)

	// PATCH attempting to flip to a different class+signal → 400.
	resp, raw = doJSON(t, http.MethodPatch, base+"/"+created.Data.ID, "", map[string]any{
		"name":     "now a loop",
		"class":    "event",
		"signal":   "loop",
		"severity": "page",
		"channel":  "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"changing class/signal on update must be rejected, body=%s", raw)
}
