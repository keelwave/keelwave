package alerting

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/alerting/channels"
	"github.com/keelwave/keelwave/internal/store"
)

const (
	workerBatch    = 20
	workerInterval = 2 * time.Second
	maxAttempts    = 5
)

// Worker drains notification_jobs and delivers each via its channel Sender,
// retrying failures with exponential backoff until maxAttempts.
type Worker struct {
	s        store.Storage
	reg      channels.Registry
	log      *zap.SugaredLogger
	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewWorker builds a delivery worker that drains notification_jobs and sends
// each via the channel registered for its channel name.
func NewWorker(s store.Storage, reg channels.Registry, log *zap.SugaredLogger) *Worker {
	return &Worker{s: s, reg: reg, log: log, done: make(chan struct{})}
}

// Start launches the drain loop in a goroutine; it exits on ctx cancel or Stop.
func (w *Worker) Start(ctx context.Context) {
	w.wg.Go(func() {
		t := time.NewTicker(workerInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-t.C:
				if _, err := w.drainOnce(ctx); err != nil {
					w.log.Warnw("notification drain failed", "err", err)
				}
			}
		}
	})
}

// Stop signals the loop to exit and waits for the goroutine to finish, returning
// ctx.Err() if the shutdown deadline fires first.
func (w *Worker) Stop(ctx context.Context) error {
	// Idempotent: a double Stop (e.g. two shutdown signals) must not panic on an
	// already-closed channel.
	w.stopOnce.Do(func() { close(w.done) })
	stopped := make(chan struct{})
	go func() { w.wg.Wait(); close(stopped) }()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drainOnce claims a batch of due jobs and dispatches each: on success MarkDone,
// on failure MarkRetry with exponential backoff (dead once attempts hit the cap),
// and unknown channels are marked dead immediately.
func (w *Worker) drainOnce(ctx context.Context) (int, error) {
	jobs, err := w.s.NotificationJobs.Claim(ctx, workerBatch)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		sender, ok := w.reg.For(j.Channel)
		if !ok {
			if err := w.s.NotificationJobs.MarkRetry(ctx, j.ID, "unknown channel", time.Now(), true); err != nil {
				w.log.Warnw("notification mark retry failed", "job", j.ID, "err", err)
			}
			continue
		}
		if err := sender.Send(ctx, j.Payload); err != nil {
			dead := j.Attempts+1 >= maxAttempts
			backoff := time.Now().Add(time.Duration(1<<j.Attempts) * time.Second)
			if merr := w.s.NotificationJobs.MarkRetry(ctx, j.ID, err.Error(), backoff, dead); merr != nil {
				w.log.Warnw("notification mark retry failed", "job", j.ID, "err", merr)
			}
			continue
		}
		if err := w.s.NotificationJobs.MarkDone(ctx, j.ID); err != nil {
			w.log.Warnw("notification mark done failed", "job", j.ID, "err", err)
		}
	}
	return len(jobs), nil
}
