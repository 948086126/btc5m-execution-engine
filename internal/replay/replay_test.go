package replay

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
)

func writeOne(t *testing.T, path, source string, mono int64, sourceSeq uint64) {
	t.Helper()
	w, err := journal.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Append(event.Record{SchemaVersion: "v1", RunID: "r", Source: source, SourceSessionID: "s", SourceReceiveSequence: sourceSeq, ReceiveMonotonicNS: mono, Kind: event.KindStateHeartbeat, TransportValidity: "VALID", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicOrdering(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a.ndjson")
	b := filepath.Join(d, "b.ndjson")
	writeOne(t, a, "PM_B", 100, 2)
	writeOne(t, b, "M2", 100, 1)
	items, err := Load([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Envelope.Event.Source != "M2" {
		t.Fatalf("unexpected order: %+v", items)
	}
}
