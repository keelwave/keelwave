package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/keelwave/keelwave/internal/batch"
	"github.com/keelwave/keelwave/internal/store"
)

type ingestAgentStepPayload struct {
	Timestamp     *time.Time      `json:"timestamp,omitempty"`
	AgentRunID    uuid.UUID       `json:"agent_run_id"  validate:"required"`
	StepIndex     int             `json:"step_index"    validate:"gte=0"`
	StepType      string          `json:"step_type"     validate:"required,oneof=think tool_call tool_result replan"`
	Content       *string         `json:"content,omitempty"      validate:"omitempty,max=100000"`
	ToolName      *string         `json:"tool_name,omitempty"    validate:"omitempty,max=200"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty" swaggertype:"object"`
	ToolOutput    json.RawMessage `json:"tool_output,omitempty" swaggertype:"object"`
	ToolSuccess   *bool           `json:"tool_success,omitempty"`
	ToolLatencyMs *int            `json:"tool_latency_ms,omitempty" validate:"omitempty,gte=0"`
	Tokens        *int            `json:"tokens,omitempty"          validate:"omitempty,gte=0"`
	CostUSD       *float64        `json:"cost_usd,omitempty"        validate:"omitempty,gte=0"`
	Metadata      json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
}

// IngestAgentStep godoc
//
//	@Summary		Append an agent step
//	@Description	Records one step in an agent loop. Server computes a SHA-256 fingerprint of tool_name + tool_input for loop detection and bumps the agent_tools registry when tool_name is present.
//	@Tags			ingest
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		ingestAgentStepPayload	true	"Step payload"
//	@Success		201		{object}	ingestResponse
//	@Failure		400		{object}	error
//	@Failure		401		{object}	error
//	@Failure		500		{object}	error
//	@Security		ApiKeyAuth
//	@Router			/ingest/agent/steps [post]
func (app *application) ingestAgentStepHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	var payload ingestAgentStepPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	step := &store.AgentStep{
		ID:               id,
		ProjectID:        projectID,
		AgentRunID:       payload.AgentRunID,
		StepIndex:        payload.StepIndex,
		StepType:         payload.StepType,
		Content:          payload.Content,
		ToolName:         payload.ToolName,
		ToolInput:        []byte(payload.ToolInput),
		ToolOutput:       []byte(payload.ToolOutput),
		ToolSuccess:      payload.ToolSuccess,
		ToolLatencyMs:    payload.ToolLatencyMs,
		Tokens:           payload.Tokens,
		CostUSD:          payload.CostUSD,
		Metadata:         []byte(payload.Metadata),
		InputFingerprint: stepFingerprint(payload.ToolName, payload.ToolInput),
	}
	if payload.Timestamp != nil {
		step.Timestamp = *payload.Timestamp
	} else {
		step.Timestamp = time.Now()
	}

	if err := app.batchers.AgentSteps.Enqueue(r.Context(), step); err != nil {
		if errors.Is(err, batch.ErrBufferFull) {
			app.serviceUnavailableResponse(w, r)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	if payload.ToolName != nil {
		go app.upsertTool(projectID, *payload.ToolName)
	}

	if err := app.jsonResponse(w, http.StatusCreated, ingestResponse{ID: &step.ID, Timestamp: step.Timestamp}); err != nil {
		app.internalServerError(w, r, err)
	}
}

func stepFingerprint(toolName *string, toolInput json.RawMessage) []byte {
	if toolName == nil {
		return nil
	}
	h := sha256.New()
	h.Write([]byte(*toolName))
	h.Write([]byte{0})
	h.Write(toolInput)
	sum := h.Sum(nil)
	return sum
}

func (app *application) upsertTool(projectID uuid.UUID, toolName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := app.store.AgentTools.UpsertSeen(ctx, projectID, toolName); err != nil {
		app.logger.Warnw("agent_tools upsert failed", "err", err, "tool_name", toolName)
	}
}

// --- Reads --------------------------------------------------------------

// ListAgentSteps godoc
//
//	@Summary	List steps for an agent run, ordered by step_index asc
//	@Tags		agent
//	@Produce	json
//	@Param		projectID	path		string	true	"Project UUID"
//	@Param		runID		path		string	true	"Agent run UUID"
//	@Param		at			query		string	false	"RFC3339 hint for chunk pruning"
//	@Param		limit		query		int		false	"default 1000, max 1000"
//	@Success	200			{array}		store.AgentStep
//	@Failure	400			{object}	error
//	@Failure	401			{object}	error
//	@Failure	500			{object}	error
//	@Security	ApiKeyAuth
//	@Router		/projects/{projectID}/agent/runs/{runID}/steps [get]
func (app *application) listAgentStepsHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	runID, err := parseUUIDParam(r, "runID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	from, to, err := parseAtWindow(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	limit, err := parseIntInRange(r.URL.Query().Get("limit"), maxStepsLimit, 1, maxStepsLimit)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	steps, err := app.store.AgentSteps.ListByRun(r.Context(), projectID, runID, from, to, limit)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, steps); err != nil {
		app.internalServerError(w, r, err)
	}
}

// ListAgentLoops godoc
//
//	@Summary		Loop detection for an agent run
//	@Description	Returns each tool_name + input fingerprint repeated >= 2 times.
//	@Tags			agent
//	@Produce		json
//	@Param			projectID	path		string	true	"Project UUID"
//	@Param			runID		path		string	true	"Agent run UUID"
//	@Param			at			query		string	false	"RFC3339 hint for chunk pruning"
//	@Success		200			{array}		store.LoopHit
//	@Failure		400			{object}	error
//	@Failure		401			{object}	error
//	@Failure		500			{object}	error
//	@Security		ApiKeyAuth
//	@Router			/projects/{projectID}/agent/runs/{runID}/loops [get]
func (app *application) listAgentLoopsHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	runID, err := parseUUIDParam(r, "runID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	from, to, err := parseAtWindow(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	hits, err := app.store.AgentSteps.ListLoops(r.Context(), projectID, runID, from, to)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, hits); err != nil {
		app.internalServerError(w, r, err)
	}
}

// StepTypeDistribution godoc
//
//	@Summary	Step counts grouped by step_type
//	@Tags		agent
//	@Produce	json
//	@Param		projectID	path	string	true	"Project UUID"
//	@Param		from		query	string	false	"RFC3339, default now - 30d"
//	@Param		to			query	string	false	"RFC3339, default now"
//	@Success	200			{array}	store.StepTypeCount
//	@Security	ApiKeyAuth
//	@Router		/projects/{projectID}/agent/steps/distribution [get]
func (app *application) stepDistributionHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	params, err := parseListParams(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	dist, err := app.store.AgentSteps.StepTypeDistribution(r.Context(), projectID, params.From, params.To)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, dist); err != nil {
		app.internalServerError(w, r, err)
	}
}

// ToolStats godoc
//
//	@Summary		Per-tool usage analytics for the calling project
//	@Description	Aggregates tool calls across every run in the window: call count, success/fail counts, success rate, and p95 latency per tool.
//	@Tags			agent
//	@Produce		json
//	@Param			projectID	path		string	true	"Project UUID"
//	@Param			from		query		string	false	"RFC3339, default now - 30d"
//	@Param			to			query		string	false	"RFC3339, default now"
//	@Success		200			{array}		store.ToolStat
//	@Failure		400			{object}	error
//	@Failure		401			{object}	error
//	@Failure		500			{object}	error
//	@Security		ApiKeyAuth
//	@Router			/projects/{projectID}/agent/tools/stats [get]
func (app *application) toolStatsHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	params, err := parseListParams(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	stats, err := app.store.AgentSteps.ToolStats(r.Context(), projectID, params.From, params.To)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusOK, stats); err != nil {
		app.internalServerError(w, r, err)
	}
}
