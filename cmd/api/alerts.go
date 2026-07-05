package main

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/keelwave/keelwave/internal/store"
)

type alertRulePayload struct {
	AgentName            *string         `json:"agent_name"`
	Name                 string          `json:"name" validate:"required,max=200"`
	Class                string          `json:"class" validate:"required,oneof=event aggregate"`
	Signal               string          `json:"signal" validate:"required,oneof=run_failure loop termination_shift cost_burn tool_failure duration_p95 eval_regression"`
	Comparator           string          `json:"comparator" validate:"omitempty,oneof=> >= < <="`
	Threshold            float64         `json:"threshold" validate:"gte=0"`
	WindowSeconds        *int            `json:"window_seconds" validate:"omitempty,gt=0"`
	Severity             string          `json:"severity" validate:"required,oneof=page warn digest"`
	ForSeconds           int             `json:"for_seconds" validate:"gte=0"`
	KeepFiringForSeconds int             `json:"keep_firing_for_seconds" validate:"gte=0"`
	CooldownSeconds      int             `json:"cooldown_seconds" validate:"gte=0"`
	MinRequests          int             `json:"min_requests" validate:"gte=0"`
	Channel              string          `json:"channel" validate:"required,oneof=email slack webhook pagerduty"`
	ChannelConfig        json.RawMessage `json:"channel_config" swaggertype:"object"`
	Enabled              bool            `json:"enabled"`
}

func (p alertRulePayload) toRule(projectID uuid.UUID) *store.AlertRule {
	cmp := p.Comparator
	if cmp == "" {
		cmp = ">"
	}
	cfg := p.ChannelConfig
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	return &store.AlertRule{
		ProjectID: projectID, AgentName: p.AgentName, Name: p.Name, Class: p.Class,
		Signal: p.Signal, Comparator: cmp, Threshold: p.Threshold, WindowSeconds: p.WindowSeconds,
		Severity: p.Severity, ForSeconds: p.ForSeconds, KeepFiringForSeconds: p.KeepFiringForSeconds,
		CooldownSeconds: p.CooldownSeconds, MinRequests: p.MinRequests, Channel: p.Channel,
		ChannelConfig: cfg, Enabled: p.Enabled,
	}
}

// authorizeProject resolves the URL-scoped project and confirms it belongs to
// the org. On failure it writes the error response and returns ok=false.
func (app *application) authorizeProject(w http.ResponseWriter, r *http.Request) (orgID, projectID uuid.UUID, ok bool) {
	orgID, err := parseUUIDParam(r, "orgID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return orgID, projectID, false
	}
	projectID, err = parseUUIDParam(r, "projectID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return orgID, projectID, false
	}

	proj, err := app.store.Projects.GetByID(r.Context(), projectID)
	if err != nil {
		switch err {
		case store.ErrNotFound:
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return orgID, projectID, false
	}
	if proj.OrganizationID != orgID {
		app.notFoundResponse(w, r, store.ErrNotFound)
		return orgID, projectID, false
	}
	return orgID, projectID, true
}

// CreateAlertRule godoc
//
//	@Summary		Creates an alert rule
//	@Description	Defines a threshold alert on an agent signal, scoped to the project. Requires admin role.
//	@Tags			admin/alert-rules
//	@Accept			json
//	@Produce		json
//	@Param			orgID		path		string				true	"Organization UUID"
//	@Param			projectID	path		string				true	"Project UUID"
//	@Param			payload		body		alertRulePayload	true	"Alert rule payload"
//	@Success		201			{object}	store.AlertRule
//	@Failure		400			{object}	error
//	@Failure		404			{object}	error
//	@Failure		500			{object}	error
//	@Router			/admin/orgs/{orgID}/projects/{projectID}/alert-rules [post]
func (app *application) createAlertRuleHandler(w http.ResponseWriter, r *http.Request) {
	_, projectID, ok := app.authorizeProject(w, r)
	if !ok {
		return
	}

	var payload alertRulePayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	rule := payload.toRule(projectID)
	if err := app.store.AlertRules.Create(r.Context(), rule); err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusCreated, rule); err != nil {
		app.internalServerError(w, r, err)
	}
}

// ListAlertRules godoc
//
//	@Summary	Lists alert rules for a project
//	@Tags		admin/alert-rules
//	@Produce	json
//	@Param		orgID		path		string	true	"Organization UUID"
//	@Param		projectID	path		string	true	"Project UUID"
//	@Success	200			{array}		store.AlertRule
//	@Failure	400			{object}	error
//	@Failure	404			{object}	error
//	@Failure	500			{object}	error
//	@Router		/admin/orgs/{orgID}/projects/{projectID}/alert-rules [get]
func (app *application) listAlertRulesHandler(w http.ResponseWriter, r *http.Request) {
	_, projectID, ok := app.authorizeProject(w, r)
	if !ok {
		return
	}

	rules, err := app.store.AlertRules.ListByProject(r.Context(), projectID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, rules); err != nil {
		app.internalServerError(w, r, err)
	}
}

// UpdateAlertRule godoc
//
//	@Summary		Updates an alert rule
//	@Description	Replaces the mutable fields of an alert rule. Requires admin role.
//	@Tags			admin/alert-rules
//	@Accept			json
//	@Produce		json
//	@Param			orgID		path		string				true	"Organization UUID"
//	@Param			projectID	path		string				true	"Project UUID"
//	@Param			ruleID		path		string				true	"Alert rule UUID"
//	@Param			payload		body		alertRulePayload	true	"Alert rule payload"
//	@Success		200			{object}	store.AlertRule
//	@Failure		400			{object}	error
//	@Failure		404			{object}	error
//	@Failure		500			{object}	error
//	@Router			/admin/orgs/{orgID}/projects/{projectID}/alert-rules/{ruleID} [patch]
func (app *application) updateAlertRuleHandler(w http.ResponseWriter, r *http.Request) {
	_, projectID, ok := app.authorizeProject(w, r)
	if !ok {
		return
	}
	ruleID, err := parseUUIDParam(r, "ruleID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload alertRulePayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	existing, err := app.store.AlertRules.GetByID(r.Context(), ruleID, projectID)
	if err != nil {
		switch err {
		case store.ErrNotFound:
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	rule := payload.toRule(projectID)
	rule.ID = existing.ID
	rule.CreatedAt = existing.CreatedAt
	// class + signal are immutable in AlertRules.Update; keep the response
	// consistent with what is actually persisted.
	rule.Class = existing.Class
	rule.Signal = existing.Signal

	if err := app.store.AlertRules.Update(r.Context(), rule); err != nil {
		switch err {
		case store.ErrNotFound:
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, rule); err != nil {
		app.internalServerError(w, r, err)
	}
}

// DeleteAlertRule godoc
//
//	@Summary		Deletes an alert rule
//	@Description	Removes an alert rule and cascades to its alert_events. Requires admin role.
//	@Tags			admin/alert-rules
//	@Param			orgID		path	string	true	"Organization UUID"
//	@Param			projectID	path	string	true	"Project UUID"
//	@Param			ruleID		path	string	true	"Alert rule UUID"
//	@Success		204
//	@Failure		400	{object}	error
//	@Failure		404	{object}	error
//	@Failure		500	{object}	error
//	@Router			/admin/orgs/{orgID}/projects/{projectID}/alert-rules/{ruleID} [delete]
func (app *application) deleteAlertRuleHandler(w http.ResponseWriter, r *http.Request) {
	_, projectID, ok := app.authorizeProject(w, r)
	if !ok {
		return
	}
	ruleID, err := parseUUIDParam(r, "ruleID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.AlertRules.Delete(r.Context(), ruleID, projectID); err != nil {
		switch err {
		case store.ErrNotFound:
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	app.noContentResponse(w)
}
