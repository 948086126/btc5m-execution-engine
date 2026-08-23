package asyncwriter

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
)

var ErrBackpressure = errors.New("writer backpressure")

type Options struct {
	QueueCapacity        int
	EnqueueTimeout       time.Duration
	ArtificialWriteDelay time.Duration // fault-injection only
}

type Writer struct {
	jw        *journal.Writer
	ch        chan event.Record
	done      chan struct{}
	mu        sync.Mutex
	firstErr  error
	opts      Options
	closeOnce sync.Once
	closeErr  error
}

func New(jw *journal.Writer, opts Options) *Writer {
	if opts.QueueCapacity <= 0 {
		opts.QueueCapacity = 1024
	}
	if opts.EnqueueTimeout <= 0 {
		opts.EnqueueTimeout = 100 * time.Millisecond
	}
	w := &Writer{jw: jw, ch: make(chan event.Record, opts.QueueCapacity), done: make(chan struct{}), opts: opts}
	go w.loop()
	return w
}

func (w *Writer) loop() {
	defer close(w.done)
	for rec := range w.ch {
		if w.opts.ArtificialWriteDelay > 0 {
			time.Sleep(w.opts.ArtificialWriteDelay)
		}
		if _, err := w.jw.Append(rec); err != nil {
			w.setErr(err)
			return
		}
	}
}

func (w *Writer) setErr(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.firstErr == nil {
		w.firstErr = err
	}
}

func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.firstErr
}

func (w *Writer) Enqueue(rec event.Record) error {
	if err := w.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(w.opts.EnqueueTimeout)
	defer timer.Stop()
	select {
	case w.ch <- rec:
		return nil
	case <-timer.C:
		err := fmt.Errorf("%w: queue=%d", ErrBackpressure, cap(w.ch))
		w.setErr(err)
		return err
	}
}

func (w *Writer) Close() error {
	w.closeOnce.Do(func() {
		close(w.ch)
		<-w.done
		w.closeErr = errors.Join(w.Err(), w.jw.Close())
	})
	return w.closeErr
}
