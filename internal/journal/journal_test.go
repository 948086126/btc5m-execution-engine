package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"btc5m-execution-engine/internal/event"
)

func sampleEvent(payload string) event.Record {
	return event.Record{
		SchemaVersion: "GATE1A_EVENT_V1", RunID: "test", Source: "PM_A", PlaneID: "A",
		SourceSessionID: "s1", SourceReceiveSequence: 140, ReceiveWallMS: 1,
		ReceiveMonotonicNS: 2, Kind: event.KindTransport, TransportValidity: "VALID",
		Payload: json.RawMessage(payload),
	}
}

func TestPayloadCannotOverrideReservedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.ndjson")
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	env, err := w.Append(sampleEvent(`{"sequence":140,"record_hash":"evil","journal_sequence":2268}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if env.Sequence != 1 {
		t.Fatalf("writer sequence overwritten: %d", env.Sequence)
	}
	if env.RecordHash == "evil" {
		t.Fatal("record hash overwritten")
	}
	res, err := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 1 {
		t.Fatalf("records=%d", res.Records)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"sequence":1`) || !strings.Contains(string(b), `"sequence":140`) {
		t.Fatal("expected writer and nested payload sequences")
	}
}

func TestCorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.ndjson")
	w, _ := NewWriter(path)
	_, _ = w.Append(sampleEvent(`{"ok":true}`))
	_ = w.Close()
	b, _ := os.ReadFile(path)
	b = []byte(strings.Replace(string(b), `"VALID"`, `"BROKEN"`, 1))
	_ = os.WriteFile(path, b, 0o600)
	if _, err := ValidateFile(path); err == nil {
		t.Fatal("corruption should fail validation")
	}
}
