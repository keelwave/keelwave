// Package pricing computes LLM call cost from token counts using a per-model
// rate catalog compiled into the binary. The catalog ships as an embedded
// JSON file so a fresh self-host install reports correct costs with no
// configuration; unknown models fall back to a client-supplied cost.
//
// Catalog source: LiteLLM's model_prices_and_context_window.json (chat models),
// converted to USD per 1,000,000 tokens. Regenerate rates.json on price changes.
// A DB-backed catalog with per-project overrides and effective-date versioning
// is the planned upgrade — see docs/notes/cost-pricing.md (T-030).
package pricing

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

//go:embed rates.json
var ratesJSON []byte

// Rate is a model's price in USD per 1,000,000 tokens.
type Rate struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// rates maps a lowercased model ID to its per-million-token USD rate.
var rates = loadRates()

func loadRates() map[string]Rate {
	var raw map[string]Rate
	if err := json.Unmarshal(ratesJSON, &raw); err != nil {
		panic("pricing: invalid embedded rates.json: " + err.Error())
	}
	m := make(map[string]Rate, len(raw))
	for k, v := range raw {
		m[strings.ToLower(k)] = v
	}
	return m
}

// snapshotSuffix matches a trailing dated snapshot: OpenAI's -YYYY-MM-DD or
// Anthropic's @YYYYMMDD. Stripping it lets an unlisted dated variant fall back
// to its base model's rate.
var snapshotSuffix = regexp.MustCompile(`(-\d{4}-\d{2}-\d{2}|@\d{8})$`)

// Cost returns the USD cost of a call and whether the model has a known rate.
// It tries an exact (case-insensitive) match first, then retries with any
// trailing dated snapshot stripped. A false ok means the caller should fall
// back to a client-supplied value.
func Cost(model string, inputTokens, outputTokens int) (cost float64, ok bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	r, ok := rates[m]
	if !ok {
		r, ok = rates[snapshotSuffix.ReplaceAllString(m, "")]
	}
	if !ok {
		return 0, false
	}
	cost = float64(inputTokens)/1e6*r.InputPerMTok + float64(outputTokens)/1e6*r.OutputPerMTok
	return cost, true
}
