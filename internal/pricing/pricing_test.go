package pricing

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCost(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		in, out int
		want    float64
		wantOK  bool
	}{
		{"gpt-4o exact", "gpt-4o", 1_000_000, 1_000_000, 2.5 + 10, true},
		{"case insensitive", "GPT-4o", 1_000_000, 0, 2.5, true},
		{"whitespace trimmed", "  claude-opus-4-8  ", 1_000_000, 1_000_000, 5 + 25, true},
		{"partial tokens", "gpt-4o", 500_000, 250_000, 2.5*0.5 + 10*0.25, true},
		{"zero tokens known model", "gpt-4o", 0, 0, 0, true},
		{"unknown model falls back", "not-a-real-model", 1000, 1000, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Cost(tt.model, tt.in, tt.out)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !approx(got, tt.want) {
				t.Fatalf("cost = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshotFallback(t *testing.T) {
	// A dated variant absent from the catalog should fall back to its base
	// model's rate when the base exists.
	base, ok := rates["gpt-4o"]
	if !ok {
		t.Skip("gpt-4o not in catalog")
	}
	got, ok := Cost("gpt-4o-2099-01-01", 1_000_000, 0)
	if !ok {
		t.Fatal("expected snapshot fallback to match base model")
	}
	if !approx(got, base.InputPerMTok) {
		t.Fatalf("cost = %v, want %v", got, base.InputPerMTok)
	}
}

func TestCatalogLoaded(t *testing.T) {
	if len(rates) < 100 {
		t.Fatalf("catalog looks empty: %d entries", len(rates))
	}
}
