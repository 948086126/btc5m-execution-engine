package mockcollector

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"btc5m-execution-engine/internal/journal"
	"btc5m-execution-engine/internal/mocksource"
)

func TestReconnectAndBoundarySentinel(t *testing.T) {
	srv := mocksource.New(mocksource.Config{Addr: "127.0.0.1:0", Source: "PM_A", RateHz: 250, DisconnectAfter: 60 * time.Millisecond})
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go func() { _ = srv.Serve() }()
	p := filepath.Join(t.TempDir(), "pm.ndjson")
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	st, err := Run(ctx, Config{URL: "ws://" + srv.Addr() + "/ws", Source: "PM_A", PlaneID: "A", RunID: "r", OutputPath: p, QueueCapacity: 256, ReconnectDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if st.Reconnects < 1 {
		t.Fatalf("reconnects=%d", st.Reconnects)
	}
	if st.EmptySideStates < 1 {
		t.Fatalf("empty-side states=%d", st.EmptySideStates)
	}
	if _, err := journal.ValidateFile(p); err != nil {
		t.Fatal(err)
	}
}
