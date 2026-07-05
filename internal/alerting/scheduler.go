package alerting

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

// Scheduler periodically evaluates every enabled aggregate rule on a fixed
// ticker. Event-class rules are driven by run-finish hooks, not this loop, so
// each tick filters to class=="aggregate".
type Scheduler struct {
	ev       *Evaluator
	s        store.Storage
	log      *zap.SugaredLogger
	interval time.Duration
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewScheduler builds a scheduler that runs ev.EvaluateAggregate for each enabled
// aggregate rule every interval.
func NewScheduler(ev *Evaluator, s store.Storage, log *zap.SugaredLogger, interval time.Duration) *Scheduler {
	return &Scheduler{ev: ev, s: s, log: log, interval: interval, done: make(chan struct{})}
}

// Start launches the evaluation loop in a goroutine; it exits on ctx cancel or Stop.
func (sc *Scheduler) Start(ctx context.Context) {
	sc.wg.Add(1)
	go func() {
		defer sc.wg.Done()
		t := time.NewTicker(sc.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-sc.done:
				return
			case <-t.C:
				sc.tick(ctx)
			}
		}
	}()
}

// tick lists enabled rules and evaluates each aggregate-class rule; a single
// rule's failure is logged and does not abort the sweep.
func (sc *Scheduler) tick(ctx context.Context) {
	rules, err := sc.s.AlertRules.ListEnabled(ctx)
	if err != nil {
		sc.log.Warnw("alert rule list failed", "err", err)
		return
	}
	for _, r := range rules {
		if r.Class != "aggregate" {
			continue
		}
		if err := sc.ev.EvaluateAggregate(ctx, r); err != nil {
			sc.log.Warnw("aggregate eval failed", "rule", r.ID, "err", err)
		}
	}
}

// Stop signals the loop to exit and waits for the goroutine to finish, returning
// ctx.Err() if the shutdown deadline fires first.
func (sc *Scheduler) Stop(ctx context.Context) error {
	close(sc.done)
	stopped := make(chan struct{})
	go func() { sc.wg.Wait(); close(stopped) }()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
