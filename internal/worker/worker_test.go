package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
)

// memStore is an in-memory queue with the same rules as the Postgres one.
type memStore struct {
	mu     sync.Mutex
	tasks  []*memTask
	events map[string][]map[string]any
}

type memTask struct {
	db.Task
	status   string
	runAfter time.Time
	msg      string
}

func (m *memStore) add(max int) *memTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &memTask{Task: db.Task{ID: string(rune('a' + len(m.tasks))), Kind: "k", MaxAttempts: max}, status: "queued"}
	m.tasks = append(m.tasks, t)
	return t
}

func (m *memStore) Claim(_ context.Context, kinds []string, _ time.Duration) (*db.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		if t.status == "queued" && !time.Now().Before(t.runAfter) {
			t.status, t.Attempts = "running", t.Attempts+1
			c := t.Task
			return &c, nil
		}
	}
	return nil, nil
}
func (m *memStore) Renew(context.Context, string, time.Duration) error { return nil }
func (m *memStore) Emit(_ context.Context, id string, ev map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[id] = append(m.events[id], ev)
	return nil
}
func (m *memStore) set(id string, fn func(*memTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		if t.ID == id {
			fn(t)
		}
	}
}
func (m *memStore) FinishTask(_ context.Context, id string, failed bool, msg string) error {
	m.set(id, func(t *memTask) {
		t.status, t.msg = "done", msg
		if failed {
			t.status = "failed"
		}
	})
	return nil
}
func (m *memStore) RetryTask(_ context.Context, id string, d time.Duration, msg string) error {
	m.set(id, func(t *memTask) { t.status, t.runAfter, t.msg = "queued", time.Now().Add(d), msg })
	return nil
}
func (m *memStore) ReleaseTask(_ context.Context, id string) error {
	m.set(id, func(t *memTask) { t.status, t.Attempts = "queued", t.Attempts-1 })
	return nil
}

type busy struct{}

func (busy) Error() string   { return "busy" }
func (busy) Retryable() bool { return true }

type badInput struct{}

func (badInput) Error() string   { return "bad input" }
func (badInput) Retryable() bool { return false }

// run starts a pool with one kind and waits until cond holds.
func run(t *testing.T, store *memStore, k Kind, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	k.Name, k.Concurrency = "k", 1
	p := New(store, k)
	p.Poll, p.Lease = 5*time.Millisecond, time.Second
	p.Backoff = func(int) time.Duration { return time.Millisecond }
	p.Start(ctx)
	defer func() { cancel(); p.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(m *memStore, t *memTask) (string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return t.status, t.Attempts
}

func TestRetriesTransientErrorsThenSucceeds(t *testing.T) {
	m := &memStore{events: map[string][]map[string]any{}}
	task := m.add(3)
	var calls int
	run(t, m, Kind{Run: func(context.Context, *db.Task, func(map[string]any)) error {
		calls++
		if calls < 3 {
			return busy{}
		}
		return nil
	}}, func() bool { s, _ := status(m, task); return s == "done" })
	if _, n := status(m, task); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
	if len(m.events[task.ID]) != 2 || m.events[task.ID][0]["step"] != "retrying" {
		t.Fatalf("expected two retrying events, got %v", m.events[task.ID])
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	m := &memStore{events: map[string][]map[string]any{}}
	task := m.add(2)
	var gaveUp string
	run(t, m, Kind{
		Run:     func(context.Context, *db.Task, func(map[string]any)) error { return busy{} },
		GiveUp:  func(_ context.Context, _ *db.Task, msg string) { gaveUp = msg },
		Message: func(error) string { return "The AI service is busy." },
	}, func() bool { s, _ := status(m, task); return s == "failed" })
	if _, n := status(m, task); n != 2 || gaveUp != "The AI service is busy." {
		t.Fatalf("attempts=%d gaveUp=%q", n, gaveUp)
	}
	evs := m.events[task.ID]
	if last := evs[len(evs)-1]; last["type"] != "error" {
		t.Fatalf("last event should be the error, got %v", last)
	}
}

func TestPermanentErrorsAreNotRetried(t *testing.T) {
	for _, err := range []error{badInput{}, Permanent(errors.New("x"))} {
		m := &memStore{events: map[string][]map[string]any{}}
		task := m.add(3)
		run(t, m, Kind{Run: func(context.Context, *db.Task, func(map[string]any)) error { return err }},
			func() bool { s, _ := status(m, task); return s == "failed" })
		if _, n := status(m, task); n != 1 {
			t.Fatalf("%v: attempts = %d, want 1", err, n)
		}
	}
}

func TestPanicFailsTheTask(t *testing.T) {
	m := &memStore{events: map[string][]map[string]any{}}
	task := m.add(3)
	run(t, m, Kind{Run: func(context.Context, *db.Task, func(map[string]any)) error { panic("boom") }},
		func() bool { s, _ := status(m, task); return s == "failed" })
}

func TestShutdownHandsTaskBack(t *testing.T) {
	m := &memStore{events: map[string][]map[string]any{}}
	task := m.add(3)
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	p := New(m, Kind{Name: "k", Concurrency: 1, Run: func(ctx context.Context, _ *db.Task, _ func(map[string]any)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}})
	p.Poll = 5 * time.Millisecond
	p.Start(ctx)
	<-started
	cancel()
	p.Wait()
	if s, n := status(m, task); s != "queued" || n != 0 {
		t.Fatalf("after shutdown: status=%s attempts=%d, want queued with the attempt not counted", s, n)
	}
}

func TestBackoff(t *testing.T) {
	if Backoff(1) != 20*time.Second || Backoff(2) != 40*time.Second || Backoff(10) != 5*time.Minute {
		t.Fatalf("backoff: %v %v %v", Backoff(1), Backoff(2), Backoff(10))
	}
}
