package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertsListURL(ts *testServer) string {
	return ts.srv.URL + "/v1/projects/" + ts.projID + "/alerts/events"
}

func TestAlertEventsHandler_ListEmptyAndValidation(t *testing.T) {
	ts := newTestServer(t)

	var listed struct {
		Data []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"data"`
	}
	resp, raw := doJSON(t, http.MethodGet, alertsListURL(ts), "", nil, &listed, ts.cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body=%s", raw)
	assert.Empty(t, listed.Data, "no alerts fired yet")

	resp, _ = doJSON(t, http.MethodGet, alertsListURL(ts)+"?state=active", "", nil, &listed, ts.cookie)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = doJSON(t, http.MethodGet, alertsListURL(ts)+"?state=bogus", "", nil, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "unknown state is rejected")

	resp, _ = doJSON(t, http.MethodGet, alertsListURL(ts)+"?limit=9999", "", nil, nil, ts.cookie)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "limit above max is clamped, not rejected")

	resp, _ = doJSON(t, http.MethodGet, alertsListURL(ts)+"?limit=notanumber", "", nil, nil, ts.cookie)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "non-numeric limit is rejected")

	resp, _ = doJSON(t, http.MethodGet, alertsListURL(ts), "", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no session, no key")
}
