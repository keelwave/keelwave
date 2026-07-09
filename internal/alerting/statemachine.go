// Package alerting holds the alert evaluation engine: pure transition logic
// (statemachine.go), rule evaluation against the store (evaluator.go), the
// ticker scheduler, and the delivery worker.
package alerting

import "time"

// Rule holds the timing knobs the machine reads: ForSeconds (entry hysteresis),
// KeepFiringForSeconds (exit hysteresis), CooldownSeconds (event suppression).
type Rule struct {
	ForSeconds           int
	KeepFiringForSeconds int
	CooldownSeconds      int
}

// Event is the persisted per-(rule,scope) alert state the machine advances.
type Event struct {
	State           string
	FirstBreachedAt *time.Time
	FiredAt         *time.Time
	RecoveringSince *time.Time
	LastFiredAt     *time.Time
}

// Eval is one tick's result: whether the rule breached, the observed value, and
// the wall clock (injected so transitions are deterministic and testable).
type Eval struct {
	Breached bool
	Value    float64
	Now      time.Time
}

// Decision is the machine's output: the next state and whether to notify.
type Decision struct {
	NextState string
	Notify    string // "" | "fire" | "resolve"
}

// NextAggregate is the condition state machine: pending -> firing -> recovering
// -> resolved, notifying only on the fire/resolve transitions.
func NextAggregate(rule Rule, ev *Event, e Eval) Decision {
	state := "inactive"
	if ev != nil {
		state = ev.State
	}
	if e.Breached {
		switch state {
		case "inactive", "resolved":
			return Decision{NextState: "pending"}
		case "pending":
			if ev.FirstBreachedAt != nil &&
				e.Now.Sub(*ev.FirstBreachedAt) >= time.Duration(rule.ForSeconds)*time.Second {
				return Decision{NextState: "firing", Notify: "fire"}
			}
			return Decision{NextState: "pending"}
		case "recovering":
			return Decision{NextState: "firing"} // re-breach before resolve: back to firing, no re-notify
		default: // firing
			return Decision{NextState: "firing"}
		}
	}
	// not breached
	switch state {
	case "firing":
		return Decision{NextState: "recovering"}
	case "recovering":
		// keep_firing_for is timed from when the condition CLEARED
		// (RecoveringSince), not the fire time. nil = not stamped yet, so stay
		// recovering rather than resolve prematurely.
		if ev.RecoveringSince != nil &&
			e.Now.Sub(*ev.RecoveringSince) >= time.Duration(rule.KeepFiringForSeconds)*time.Second {
			return Decision{NextState: "resolved", Notify: "resolve"}
		}
		return Decision{NextState: "recovering"}
	case "pending":
		return Decision{NextState: "resolved"} // breach cleared before firing: silently drop
	default:
		return Decision{NextState: state}
	}
}

// NextEvent handles point-in-time event rules: notify once, then suppress within
// the cooldown window. No resolve.
func NextEvent(rule Rule, ev *Event, e Eval) Decision {
	if !e.Breached {
		return Decision{NextState: "fired"}
	}
	if ev != nil && ev.LastFiredAt != nil &&
		e.Now.Sub(*ev.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
		return Decision{NextState: "fired"} // within cooldown: suppress
	}
	return Decision{NextState: "fired", Notify: "fire"}
}
