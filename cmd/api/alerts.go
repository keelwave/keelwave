package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"

	"github.com/google/uuid"

	"github.com/keelwave/keelwave/internal/alerting"
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
	Channel              string          `json:"channel" validate:"required,oneof=email"`
	ChannelConfig        json.RawMessage `json:"channel_config" swaggertype:"object"`
	Enabled              bool            `json:"enabled"`
}

func (p alertRulePayload) toRule(projectID uuid.UUID) *store.AlertRule {
	cmp := p.Comparator
	if cmp == "" {
		// Aggregate rules default to the signal's natural direction (lower-is-bad
		// signals compare "<"); event rules ignore the comparator entirely.
		cmp = naturalComparator(p.Signal)
	}
	// Mirror the schema's DEFAULT 900 (15 min, spec §4.2): an omitted
	// cooldown_seconds zero-values to 0, and Create always binds it, so a 0 here
	// would make an event rule fire on every finishing run.
	cooldown := p.CooldownSeconds
	if cooldown == 0 {
		cooldown = 900
	}
	cfg := p.ChannelConfig
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	return &store.AlertRule{
		ProjectID: projectID, AgentName: p.AgentName, Name: p.Name, Class: p.Class,
		Signal: p.Signal, Comparator: cmp, Threshold: p.Threshold, WindowSeconds: p.WindowSeconds,
		Severity: p.Severity, ForSeconds: p.ForSeconds, KeepFiringForSeconds: p.KeepFiringForSeconds,
		CooldownSeconds: cooldown, MinRequests: p.MinRequests, Channel: p.Channel,
		ChannelConfig: cfg, Enabled: p.Enabled,
	}
}

// validAlertClassSignal enforces the spec §4.2 class×signal matrix: loop is
// event-only, the aggregate-window signals are aggregate-only, and run_failure
// is valid for both.
func validAlertClassSignal(class, signal string) bool {
	switch class {
	case "event":
		switch signal {
		case "loop", "run_failure":
			return true
		}
	case "aggregate":
		switch signal {
		case "run_failure", "termination_shift", "cost_burn", "tool_failure", "duration_p95", "eval_regression":
			return true
		}
	}
	return false
}

// lowerIsBad reports whether a signal's metric is "bad" when it falls (completion
// rate, correctness), so the natural comparator is "<". The rest are higher-is-bad
// (cost, failure/bad-termination share, latency), natural comparator ">".
func lowerIsBad(signal string) bool {
	switch signal {
	case "run_failure", "eval_regression":
		return true
	default:
		return false
	}
}

// naturalComparator is the default comparator for an aggregate signal in its
// natural alert direction.
func naturalComparator(signal string) string {
	if lowerIsBad(signal) {
		return "<"
	}
	return ">"
}

// validAggregateComparator rejects a comparator that points the wrong way for the
// signal: lower-is-bad signals may only use "<"/"<=", higher-is-bad only ">"/">=".
// An empty comparator is allowed (toRule fills the natural default).
func validAggregateComparator(signal, comparator string) bool {
	if comparator == "" {
		return true
	}
	if lowerIsBad(signal) {
		return comparator == "<" || comparator == "<="
	}
	return comparator == ">" || comparator == ">="
}

// validateAlertRulePayload runs the cross-field checks not expressible as struct
// tags: the class×signal matrix, aggregate comparator direction, and the email
// channel's required recipient. Returns a non-nil error to surface as a 400.
func validateAlertRulePayload(p alertRulePayload) error {
	if !validAlertClassSignal(p.Class, p.Signal) {
		return fmt.Errorf("signal %q is not valid for class %q", p.Signal, p.Class)
	}
	if p.Class == "aggregate" && !validAggregateComparator(p.Signal, p.Comparator) {
		return fmt.Errorf("comparator %q points the wrong way for signal %q (want %q direction)",
			p.Comparator, p.Signal, naturalComparator(p.Signal))
	}
	if p.Channel == "email" {
		if err := validateEmailRecipient(p.ChannelConfig); err != nil {
			return err
		}
	}
	return nil
}

// validateEmailRecipient requires channel_config to carry a parseable email
// address in "to". A dead recipient would otherwise fail every delivery and
// dead-letter silently.
func validateEmailRecipient(cfg json.RawMessage) error {
	var c struct {
		To string `json:"to"`
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &c); err != nil {
			return fmt.Errorf("channel_config must be a JSON object: %w", err)
		}
	}
	if _, err := mail.ParseAddress(c.To); err != nil {
		return fmt.Errorf("email channel requires channel_config.to to be a valid address")
	}
	return nil
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
	if err := validateAlertRulePayload(payload); err != nil {
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
	if err := validateAlertRulePayload(payload); err != nil {
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

	// class + signal are immutable in AlertRules.Update; a body that tries to
	// change them was validated under the wrong class and must be rejected rather
	// than silently ignored.
	if payload.Class != existing.Class || payload.Signal != existing.Signal {
		app.badRequestResponse(w, r, fmt.Errorf("class and signal are immutable; got class %q signal %q, have class %q signal %q",
			payload.Class, payload.Signal, existing.Class, existing.Signal))
		return
	}

	rule := payload.toRule(projectID)
	rule.ID = existing.ID
	rule.CreatedAt = existing.CreatedAt
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

type alertRulePreviewPayload struct {
	AgentName     *string `json:"agent_name,omitempty" validate:"omitempty,max=200"`
	Signal        string  `json:"signal" validate:"required,oneof=run_failure loop termination_shift cost_burn tool_failure duration_p95 eval_regression"`
	Comparator    string  `json:"comparator" validate:"required,oneof=> >= < <="`
	Threshold     float64 `json:"threshold"`
	WindowSeconds *int    `json:"window_seconds,omitempty" validate:"omitempty,gt=0"`
	MinRequests   int     `json:"min_requests" validate:"gte=0"`
}

type alertRulePreviewResult struct {
	Value       float64 `json:"value"`
	SampleCount int     `json:"sample_count"`
	WouldBreach bool    `json:"would_breach"`
	ScopeLabel  string  `json:"scope_label"`
}

// PreviewAlertRule godoc
//
//	@Summary		Preview an unsaved alert rule
//	@Description	Evaluates a draft rule's signal over its window right now and reports whether the threshold would breach. Aggregate signals only.
//	@Tags			admin/alert-rules
//	@Accept			json
//	@Produce		json
//	@Param			orgID		path		string					true	"Organization UUID"
//	@Param			projectID	path		string					true	"Project UUID"
//	@Param			payload		body		alertRulePreviewPayload	true	"Draft rule"
//	@Success		200			{object}	alertRulePreviewResult
//	@Failure		400			{object}	error
//	@Failure		401			{object}	error
//	@Failure		403			{object}	error
//	@Failure		404			{object}	error
//	@Failure		500			{object}	error
//	@Router			/admin/orgs/{orgID}/projects/{projectID}/alert-rules/preview [post]
func (app *application) previewAlertRuleHandler(w http.ResponseWriter, r *http.Request) {
	_, projectID, ok := app.authorizeProject(w, r)
	if !ok {
		return
	}

	var payload alertRulePreviewPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if payload.Signal == "loop" {
		app.badRequestResponse(w, r, errors.New("loop is an event signal and has no metric to preview"))
		return
	}
	// A draft the create endpoint would reject must not preview as valid.
	if !validAggregateComparator(payload.Signal, payload.Comparator) {
		app.badRequestResponse(w, r, fmt.Errorf("comparator %q points the wrong way for signal %q", payload.Comparator, payload.Signal))
		return
	}
	if app.evaluator == nil {
		app.internalServerError(w, r, errors.New("alerting evaluator not configured"))
		return
	}

	rule := &store.AlertRule{
		ProjectID:     projectID,
		AgentName:     payload.AgentName,
		Class:         "aggregate",
		Signal:        payload.Signal,
		Comparator:    payload.Comparator,
		Threshold:     payload.Threshold,
		WindowSeconds: payload.WindowSeconds,
		MinRequests:   payload.MinRequests,
	}

	value, scope, count, err := app.evaluator.Preview(r.Context(), rule)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	result := alertRulePreviewResult{
		Value:       value,
		SampleCount: count,
		WouldBreach: alerting.Breached(value, count, rule),
		ScopeLabel:  scope,
	}
	if err := app.jsonResponse(w, http.StatusOK, result); err != nil {
		app.internalServerError(w, r, err)
	}
}
