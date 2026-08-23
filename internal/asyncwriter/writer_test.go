package asyncwriter

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
)

func TestBackpressureFailsClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bp.ndjson")
	jw, err := journal.NewWriter(p)
	if err != nil {
		t.Fatal(err)
	}
	w := New(jw, Options{QueueCapacity: 1, EnqueueTimeout: 2 * time.Millisecond, ArtificialWriteDelay: 100 * time.Millisecond})
	rec := event.Record{SchemaVersion: "v1", RunID: "r", Source: "M2", SourceSessionID: "s", SourceReceiveSequence: 1, ReceiveMonotonicNS: 1, Kind: event.KindM2Quote, TransportValidity: "VALID", Payload: json.RawMessage(`{}`)}
	var got error
	for i := 0; i < 20; i++ {
		if err := w.Enqueue(rec); err != nil {
			got = err
			break
		}
	}
	_ = w.Close()
	if !errors.Is(got, ErrBackpressure) {
		t.Fatalf("got %v", got)
	}
	if _, err := journal.ValidateFile(p); err != nil {
		t.Fatalf("journal should remain internally valid: %v", err)
	}
}
