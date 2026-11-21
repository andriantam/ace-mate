// Package worker runs background tasks from the Postgres queue (see db.Task).
//
// Each server instance runs a few workers per kind of task. A worker claims a task with a
// lease and renews it while it works; if the server dies, the lease runs out and another
// worker (on this instance after a restart, or on another instance) picks the task up.
// Failures that may pass (the AI service is busy or unreachable) are retried with a
// growing delay; anything else fails at once with a message for the user.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
)

// Store is the part of the database the workers use.
type Store interface {
	Claim(ctx context.Context, kinds []string, lease time.Duration) (*db.Task, error)
	Renew(ctx context.Context, id string, lease time.Duration) error
	Emit(ctx context.Context, taskID string, ev map[string]any) error
	FinishTask(ctx context.Context, id string, failed bool, msg string) error
	RetryTask(ctx context.Context, id string, delay time.Duration, msg string) error
	ReleaseTask(ctx context.Context, id string) error
}

// Kind is one type of task and how to run it.
type Kind struct {
	Name        string
	Concurrency int
	// Run does the work. emit records progress events for the page that started it.
	Run func(ctx context.Context, t *db.Task, emit func(map[string]any)) error
	// GiveUp records a final failure on the task's document or job.
	GiveUp func(ctx context.Context, t *db.Task, msg string)
	// Message turns an error into something safe to show the user.
	Message func(error) string
}

// Retryable is implemented by errors that know whether trying again may help.
type Retryable interface{ Retryable() bool }

// ShouldRetry reports whether a failed task is worth another attempt. Errors that don't
// say (a database hiccup, an unexpected failure) are retried.
func ShouldRetry(err error) bool {
	var r Retryable
	if errors.As(err, &r) {
		return r.Retryable()
	}
	return true
}

// Pool runs the workers.
type Pool struct {
	store Store
	kinds map[string]Kind
	wake  map[string]chan struct{}
	wg    sync.WaitGroup

	Lease   time.Duration                   // how long a claim lasts without renewal
	Poll    time.Duration                   // how often idle workers look for work
	Backoff func(attempt int) time.Duration // delay before retrying after a failed attempt
}

func New(store Store, kinds ...Kind) *Pool {
	p := &Pool{
		store: store, kinds: map[string]Kind{}, wake: map[string]chan struct{}{},
		Lease: 2 * time.Minute, Poll: 2 * time.Second, Backoff: Backoff,
	}
	for _, k := range kinds {
		p.kinds[k.Name] = k
		p.wake[k.Name] = make(chan struct{}, 1)
	}
	return p
}

// Backoff waits 20s, 40s, 80s… up to 5 minutes between attempts.
func Backoff(attempt int) time.Duration {
	d := time.Duration(20*math.Pow(2, float64(attempt-1))) * time.Second
	return min(d, 5*time.Minute)
}

// Wake tells an idle worker of this kind to look for work now, instead of at its next poll.
func (p *Pool) Wake(kind string) {
	select {
	case p.wake[kind] <- struct{}{}:
	default:
	}
}

// Start runs the workers until ctx is cancelled. Wait blocks until they have stopped.
func (p *Pool) Start(ctx context.Context) {
	for _, k := range p.kinds {
		for range max(1, k.Concurrency) {
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.loop(ctx, k)
			}()
		}
	}
}

func (p *Pool) Wait() { p.wg.Wait() }

func (p *Pool) loop(ctx context.Context, k Kind) {
	for ctx.Err() == nil {
		t, err := p.store.Claim(ctx, []string{k.Name}, p.Lease)
		if err != nil && ctx.Err() == nil {
			slog.Warn("worker: claim", "kind", k.Name, "err", err)
		}
		if t != nil {
			p.run(ctx, k, t)
			continue
		}
		select {
		case <-ctx.Done():
		case <-p.wake[k.Name]:
		case <-time.After(p.Poll):
		}
	}
}

func (p *Pool) run(ctx context.Context, k Kind, t *db.Task) {
	// Bookkeeping must still happen while the server shuts down.
	bg := context.WithoutCancel(ctx)
	emit := func(ev map[string]any) {
		if err := p.store.Emit(bg, t.ID, ev); err != nil {
			slog.Warn("worker: emit", "task", t.ID, "err", err)
		}
	}
	giveUp := func(msg string) {
		if k.GiveUp != nil {
			k.GiveUp(bg, t, msg)
		}
		emit(map[string]any{"type": "error", "error": msg})
		_ = p.store.FinishTask(bg, t.ID, true, msg)
	}
	if t.Attempts > t.MaxAttempts {
		giveUp("This was interrupted too many times. Please try again.")
		return
	}

	err := p.runWithLease(ctx, k, t, emit)
	switch {
	case err == nil:
		_ = p.store.FinishTask(bg, t.ID, false, "")
	case ctx.Err() != nil:
		// Shutting down: hand the task back so the next start (or another instance) runs it.
		_ = p.store.ReleaseTask(bg, t.ID)
	default:
		msg := message(k, err)
		if ShouldRetry(err) && t.Attempts < t.MaxAttempts {
			delay := p.Backoff(t.Attempts)
			slog.Info("worker: retrying", "kind", k.Name, "task", t.ID, "attempt", t.Attempts, "in", delay, "err", err)
			emit(map[string]any{"type": "progress", "step": "retrying", "detail": msg,
				"attempt": t.Attempts + 1, "of": t.MaxAttempts, "seconds": int(delay.Seconds())})
			_ = p.store.RetryTask(bg, t.ID, delay, err.Error())
			return
		}
		slog.Warn("worker: failed", "kind", k.Name, "task", t.ID, "attempt", t.Attempts, "err", err)
		giveUp(msg)
	}
}

// runWithLease runs the task, renewing its lease until it returns.
func (p *Pool) runWithLease(ctx context.Context, k Kind, t *db.Task, emit func(map[string]any)) (err error) {
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(p.Lease / 3)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if err := p.store.Renew(context.WithoutCancel(ctx), t.ID, p.Lease); err != nil {
					slog.Warn("worker: renew lease", "task", t.ID, "err", err)
				}
			}
		}
	}()
	defer func() {
		if v := recover(); v != nil {
			slog.Error("worker: panic", "kind", k.Name, "task", t.ID, "value", v)
			err = permanent{fmt.Errorf("panic: %v", v)}
		}
	}()
	return k.Run(ctx, t, emit)
}

func message(k Kind, err error) string {
	if k.Message != nil {
		return k.Message(err)
	}
	return "Something went wrong."
}

// permanent marks an error as not worth retrying.
type permanent struct{ error }

func (permanent) Retryable() bool { return false }

// Permanent wraps err so the task fails without retrying.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

func (p permanent) Unwrap() error { return p.error }
