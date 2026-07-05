package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/keelwave/keelwave/internal/batch"
	"github.com/keelwave/keelwave/internal/pricing"
	"github.com/keelwave/keelwave/internal/store"
)

type ingestAIPayload struct {
	Timestamp    *time.Time      `json:"timestamp,omitempty"`
	Model        string          `json:"model"         validate:"required,min=1,max=200"`
	Provider     *string         `json:"provider,omitempty"      validate:"omitempty,max=50"`
	InputTokens  *int            `json:"input_tokens,omitempty"  validate:"omitempty,gte=0"`
	OutputTokens *int            `json:"output_tokens,omitempty" validate:"omitempty,gte=0"`
	TotalTokens  *int            `json:"total_tokens,omitempty"  validate:"omitempty,gte=0"`
	CostUSD      *float64        `json:"cost_usd,omitempty"      validate:"omitempty,gte=0"`
	LatencyMs    *int            `json:"latency_ms,omitempty"    validate:"omitempty,gte=0"`
	Status       string          `json:"status"        validate:"required,oneof=success error timeout"`
	ErrorMessage *string         `json:"error_message,omitempty" validate:"omitempty,max=2000"`
	RequestID    *string         `json:"request_id,omitempty"    validate:"omitempty,max=200"`
	AgentRunID   *uuid.UUID      `json:"agent_run_id,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
}

// IngestAI godoc
//
//	@Summary		Ingest an AI / LLM call trace
//	@Description	Records one LLM call: model, token counts, cost, latency, status. project_id is derived from the API key.
//	@Tags			ingest
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		ingestAIPayload	true	"AI trace payload"
//	@Success		201		{object}	ingestResponse
//	@Failure		400		{object}	error
//	@Failure		401		{object}	error
//	@Failure		500		{object}	error
//	@Security		ApiKeyAuth
//	@Router			/ingest/ai [post]
func (app *application) ingestAIHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())

	var payload ingestAIPayload
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

	// Compute cost server-side from the catalog; trust the client value only
	// when the model has no known rate.
	cost := payload.CostUSD
	if payload.InputTokens != nil && payload.OutputTokens != nil {
		if c, ok := pricing.Cost(payload.Model, *payload.InputTokens, *payload.OutputTokens); ok {
			cost = &c
		}
	}

	t := &store.AITrace{
		ID:           id,
		ProjectID:    projectID,
		Model:        payload.Model,
		Provider:     payload.Provider,
		InputTokens:  payload.InputTokens,
		OutputTokens: payload.OutputTokens,
		TotalTokens:  payload.TotalTokens,
		CostUSD:      cost,
		LatencyMs:    payload.LatencyMs,
		Status:       payload.Status,
		ErrorMessage: payload.ErrorMessage,
		RequestID:    payload.RequestID,
		AgentRunID:   payload.AgentRunID,
		Metadata:     []byte(payload.Metadata),
	}
	if payload.Timestamp != nil {
		t.Timestamp = *payload.Timestamp
	} else {
		t.Timestamp = time.Now()
	}

	if err := app.batchers.AITraces.Enqueue(r.Context(), t); err != nil {
		if errors.Is(err, batch.ErrBufferFull) {
			app.serviceUnavailableResponse(w, r)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusCreated, ingestResponse{ID: &t.ID, Timestamp: t.Timestamp}); err != nil {
		app.internalServerError(w, r, err)
	}
}
