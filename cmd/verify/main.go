package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc5m-execution-engine/internal/asyncwriter"
	"btc5m-execution-engine/internal/clock"
	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
	"btc5m-execution-engine/internal/polymarket"
)

func main() {
	ok := true
	if v, err := polymarket.NormalizeEmbeddedBest("0", "bid", "0.01"); err != nil || v != nil {
		fmt.Println("R2_BOUNDARY_CASE_FAIL", err)
		ok = false
	} else if v, err := polymarket.NormalizeEmbeddedBest("1", "ask", "0.01"); err != nil || v != nil {
		fmt.Println("R2_BOUNDARY_CASE_FAIL", err)
		ok = false
	} else {
		fmt.Println("R2_BOUNDARY_CASE_PASS")
	}

	d, err := os.MkdirTemp("", "btc5m-verify-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(d)

	path := filepath.Join(d, "control.ndjson")
	w, err := journal.NewWriter(path)
	if err != nil {
		panic(err)
	}
	payload := json.RawMessage(`{"sequence":140,"record_hash":"evil","journal_sequence":2268}`)
	env, err := w.Append(event.Record{SchemaVersion: "GATE1A_EVENT_V1", RunID: "verify", Source: "CONTROL", SourceSessionID: "s", SourceReceiveSequence: 140, ReceiveMonotonicNS: 1, Kind: event.KindTransport, TransportValidity: "VALID", Payload: payload})
	if err != nil {
		panic(err)
	}
	_ = w.Close()
	if env.Sequence != 1 || env.RecordHash == "evil" {
		fmt.Println("R3_COLLISION_CASE_FAIL")
		ok = false
	} else if _, err := journal.ValidateFile(path); err != nil {
		fmt.Println("JOURNAL_INTEGRITY_FAIL", err)
		ok = false
	} else {
		fmt.Println("R3_COLLISION_CASE_PASS")
		fmt.Println("JOURNAL_INTEGRITY_PASS")
	}

	// Deliberately corrupt a copy and require the verifier to reject it.
	corrupt := filepath.Join(d, "corrupt.ndjson")
	b, _ := os.ReadFile(path)
	b = []byte(strings.Replace(string(b), `"VALID"`, `"BROKEN"`, 1))
	_ = os.WriteFile(corrupt, b, 0o600)
	if _, err := journal.ValidateFile(corrupt); err == nil {
		fmt.Println("CORRUPTION_DETECTED_FAIL")
		ok = false
	} else {
		fmt.Println("CORRUPTION_DETECTED_PASS")
	}

	a, err := clock.MonotonicNS()
	if err != nil {
		fmt.Println("MONOTONIC_CLOCK_FAIL", err)
		ok = false
	} else {
		b, err := clock.MonotonicNS()
		if err != nil || b < a {
			fmt.Println("MONOTONIC_CLOCK_FAIL", err)
			ok = false
		} else {
			fmt.Println("MONOTONIC_CLOCK_PASS")
		}
	}

	// Backpressure must become an explicit fail-closed error, never a silent drop.
	bpPath := filepath.Join(d, "backpressure.ndjson")
	jw, err := journal.NewWriter(bpPath)
	if err != nil {
		panic(err)
	}
	aw := asyncwriter.New(jw, asyncwriter.Options{QueueCapacity: 1, EnqueueTimeout: 2 * time.Millisecond, ArtificialWriteDelay: 100 * time.Millisecond})
	rec := event.Record{SchemaVersion: "GATE1A_EVENT_V1", RunID: "verify", Source: "M2", SourceSessionID: "s", SourceReceiveSequence: 1, ReceiveMonotonicNS: 1, Kind: event.KindM2Quote, TransportValidity: "VALID", Payload: json.RawMessage(`{}`)}
	var bpErr error
	for i := 0; i < 20; i++ {
		if err := aw.Enqueue(rec); err != nil {
			bpErr = err
			break
		}
	}
	_ = aw.Close()
	if !errors.Is(bpErr, asyncwriter.ErrBackpressure) {
		fmt.Println("BACKPRESSURE_FAIL_CLOSED_FAIL", bpErr)
		ok = false
	} else if _, err := journal.ValidateFile(bpPath); err != nil {
		fmt.Println("BACKPRESSURE_JOURNAL_FAIL", err)
		ok = false
	} else {
		fmt.Println("BACKPRESSURE_FAIL_CLOSED_PASS")
	}

	if !ok {
		os.Exit(1)
	}
	fmt.Println("ALL_TESTS_PASS")
}
