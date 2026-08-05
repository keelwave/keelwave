package main

import (
	"errors"
	"net/http"
)

const (
	defaultAlertsLimit = 50
	maxAlertsLimit     = 200
)

// ListAlerts godoc
//
//	@Summary		List alerts for a project
//	@Description	Fired alert instances newest-first, each with its latest notification delivery status. state=active covers pending, firing and recovering.
//	@Tags			alerts
//	@Produce		json
//	@Param			projectID	path		string	true	"Project UUID"
//	@Param			state		query		string	false	"active | resolved (default: all)"
//	@Param			limit		query		int		false	"default 50, max 200"
//	@Success		200			{array}		store.AlertEventWithDelivery
//	@Failure		400			{object}	error
//	@Failure		401			{object}	error
//	@Failure		500			{object}	error
//	@Security		ApiKeyAuth
//	@Router			/projects/{projectID}/alerts/events [get]
func (app *application) listAlertsHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	state := r.URL.Query().Get("state")
	switch state {
	case "", "active", "resolved":
	default:
		app.badRequestResponse(w, r, errors.New("state must be active or resolved"))
		return
	}

	limit, err := parseIntInRange(r.URL.Query().Get("limit"), defaultAlertsLimit, 1, maxAlertsLimit)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	alerts, err := app.store.AlertEvents.ListByProjectFiltered(r.Context(), projectID, state, limit)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, alerts); err != nil {
		app.internalServerError(w, r, err)
	}
}
