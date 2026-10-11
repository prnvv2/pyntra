package database

import (
	"context"
	"sync"
)

// SerialWriter serializes database write operations through a single goroutine
// (docs/DESIGN-concurrent-orchestration-and-cve-cvss.md §A.1/§A.2 "single
// DB-writer goroutine"). It is used by the fanout fan-in stage, where many
// concurrent depth workers would otherwise contend on the same tables. Combined
// with WAL + busy_timeout on the connection, it gives the fan-in path a single,
// well-ordered writer.
//
// Each submitted job is a closure that performs its writes against the DB and
// returns an error. Jobs run strictly one-at-a-time, in submission order.
type SerialWriter struct {
	db    *DB
	jobs  chan writeJob
	once  sync.Once
	done  chan struct{}
	start sync.Once
}

type writeJob struct {
	fn    func(*DB) error
	resCh chan error
}

// NewSerialWriter creates a writer with a buffered job queue and starts its
// goroutine. Call Close to stop it (idempotent).
func NewSerialWriter(db *DB, queue int) *SerialWriter {
	if queue <= 0 {
		queue = 64
	}
	w := &SerialWriter{
		db:   db,
		jobs: make(chan writeJob, queue),
		done: make(chan struct{}),
	}
	w.start.Do(func() { go w.run() })
	return w
}

func (w *SerialWriter) run() {
	for job := range w.jobs {
		err := job.fn(w.db)
		job.resCh <- err
	}
	close(w.done)
}

// Do submits a write job and waits for it to complete, honoring ctx cancellation
// while queuing. Jobs already accepted run to completion.
func (w *SerialWriter) Do(ctx context.Context, fn func(*DB) error) error {
	resCh := make(chan error, 1)
	select {
	case w.jobs <- writeJob{fn: fn, resCh: resCh}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-resCh:
		return err
	case <-ctx.Done():
		// The job may still run; we stop waiting on cancellation.
		return ctx.Err()
	}
}

// CreateVulnerability serializes a finding insert through the writer.
func (w *SerialWriter) CreateVulnerability(ctx context.Context, v *Vulnerability) error {
	return w.Do(ctx, func(db *DB) error {
		_, err := db.CreateVulnerability(v)
		return err
	})
}

// UpdateVulnerability serializes a finding update through the writer.
func (w *SerialWriter) UpdateVulnerability(ctx context.Context, id string, v *Vulnerability) error {
	return w.Do(ctx, func(db *DB) error {
		return db.UpdateVulnerability(id, v)
	})
}

// Close stops the writer goroutine after draining queued jobs. Safe to call more
// than once.
func (w *SerialWriter) Close() {
	w.once.Do(func() { close(w.jobs) })
	<-w.done
}
