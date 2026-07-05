package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertRulesURL(ts *testServer) string {
	return ts.srv.URL + "/v1/admin/orgs/" + ts.orgID + "/projects/" + ts.projID + "/alert-rules"
}

func TestAlertRuleHandler_CreateListDelete(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	var created struct {
		Data struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
			Name      string `json:"name"`
			Class     string `json:"class"`
			Signal    string `json:"signal"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "loop page",
		"class":    "event",
		"signal":   "loop",
		"severity": "page",
		"channel":  "email",
		"enabled":  true,
	}, &created, ts.cookie)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body=%s", raw)
	assert.NotEmpty(t, created.Data.ID)
	assert.Equal(t, ts.projID, created.Data.ProjectID, "project_id comes from URL, not body")
	assert.Equal(t, "loop page", created.Data.Name)

	var listed struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	resp, raw = doJSON(t, http.MethodGet, base, "", nil, &listed, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	found := false
	for _, r := range listed.Data {
		if r.ID == created.Data.ID {
			found = true
			assert.Equal(t, "loop page", r.Name)
		}
	}
	assert.True(t, found, "created rule appears in list")

	ruleURL := base + "/" + created.Data.ID
	resp, _ = doJSON(t, http.MethodDelete, ruleURL, "", nil, nil, ts.cookie)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, _ = doJSON(t, http.MethodDelete, ruleURL, "", nil, nil, ts.cookie)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "second delete is 404")
}

func TestAlertRuleHandler_UpdateAppliesFields(t *testing.T) {
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
		"severity":  "warn",
		"channel":   "email",
		"threshold": 5,
		"enabled":   true,
	}, &created, ts.cookie)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body=%s", raw)

	var updated struct {
		Data struct {
			Name     string `json:"name"`
			Severity string `json:"severity"`
			Enabled  bool   `json:"enabled"`
		} `json:"data"`
	}
	resp, raw = doJSON(t, http.MethodPatch, base+"/"+created.Data.ID, "", map[string]any{
		"name":      "cost burn v2",
		"class":     "aggregate",
		"signal":    "cost_burn",
		"severity":  "page",
		"channel":   "email",
		"threshold": 10,
		"enabled":   false,
	}, &updated, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	assert.Equal(t, "cost burn v2", updated.Data.Name)
	assert.Equal(t, "page", updated.Data.Severity)
	assert.False(t, updated.Data.Enabled)
}

func TestAlertRuleHandler_ChannelConfigRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	var created struct {
		Data struct {
			ID            string          `json:"id"`
			ChannelConfig json.RawMessage `json:"channel_config"`
		} `json:"data"`
	}
	// channel_config is sent as a real JSON object, not a base64 string.
	resp, raw := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "email ops",
		"class":    "event",
		"signal":   "run_failure",
		"severity": "page",
		"channel":  "email",
		"channel_config": map[string]any{
			"to": "ops@acme.com",
		},
		"enabled": true,
	}, &created, ts.cookie)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body=%s", raw)
	require.NotEmpty(t, created.Data.ID)

	// The create response must echo the object back verbatim, not base64.
	var createdCfg struct {
		To string `json:"to"`
	}
	require.NoError(t, json.Unmarshal(created.Data.ChannelConfig, &createdCfg),
		"channel_config should be a JSON object, got %s", created.Data.ChannelConfig)
	assert.Equal(t, "ops@acme.com", createdCfg.To)

	// GET (via list) must round-trip the same JSON object back.
	var listed struct {
		Data []struct {
			ID            string          `json:"id"`
			ChannelConfig json.RawMessage `json:"channel_config"`
		} `json:"data"`
	}
	resp, raw = doJSON(t, http.MethodGet, base, "", nil, &listed, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)

	found := false
	for _, r := range listed.Data {
		if r.ID == created.Data.ID {
			found = true
			var gotCfg struct {
				To string `json:"to"`
			}
			require.NoError(t, json.Unmarshal(r.ChannelConfig, &gotCfg),
				"channel_config should be a JSON object, got %s", r.ChannelConfig)
			assert.Equal(t, "ops@acme.com", gotCfg.To)
		}
	}
	assert.True(t, found, "created rule appears in list")
}

func TestAlertRuleHandler_rejectsInvalidEnum(t *testing.T) {
	ts := newTestServer(t)
	base := alertRulesURL(ts)

	resp, _ := doJSON(t, http.MethodPost, base, "", map[string]any{
		"name":     "bad signal",
		"class":    "event",
		"signal":   "not_a_signal",
		"severity": "page",
		"channel":  "email",
	}, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid enum rejected before DB")
}
