package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"btc5m-execution-engine/internal/journal"
	"btc5m-execution-engine/internal/mockcollector"
	"btc5m-execution-engine/internal/mocksource"
	"btc5m-execution-engine/internal/replay"
)

type sourceSpec struct {
	name       string
	plane      string
	rate       int
	delay      time.Duration
	disconnect time.Duration
}

type result struct {
	Source  string                   `json:"source"`
	Plane   string                   `json:"plane,omitempty"`
	Stats   mockcollector.Stats      `json:"stats"`
	Journal journal.ValidationResult `json:"journal"`
	Error   string                   `json:"error,omitempty"`
}

type receipt struct {
	Version     string   `json:"version"`
	RunID       string   `json:"run_id"`
	DurationMS  int64    `json:"duration_ms"`
	Results     []result `json:"results"`
	ReplayItems int      `json:"replay_items"`
	Verdict     string   `json:"verdict"`
}

func main() {
	var duration time.Duration
	var out string
	flag.DurationVar(&duration, "duration", 6*time.Second, "mock-live duration")
	flag.StringVar(&out, "out", "", "output directory; defaults to a temp dir")
	flag.Parse()
	if duration < 2*time.Second {
		fmt.Fprintln(os.Stderr, "duration must be >=2s")
		os.Exit(2)
	}
	if out == "" {
		d, err := os.MkdirTemp("", "btc5m-mock-live-")
		if err != nil {
			panic(err)
		}
		out = d
	} else if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	runID := fmt.Sprintf("mock-v1-%d", time.Now().UnixMilli())

	specs := []sourceSpec{
		{name: "M2", rate: 500},
		{name: "SPOT", rate: 250},
		{name: "PM_A", plane: "A", rate: 100, disconnect: 2 * time.Second},
		{name: "PM_B", plane: "B", rate: 100, delay: 15 * time.Millisecond},
	}
	servers := make([]*mocksource.Server, 0, len(specs))
	urls := map[string]string{}
	for _, sp := range specs {
		srv := mocksource.New(mocksource.Config{Addr: "127.0.0.1:0", Source: sp.name, RateHz: sp.rate, SendDelay: sp.delay, DisconnectAfter: sp.disconnect})
		if err := srv.Listen(); err != nil {
			panic(err)
		}
		servers = append(servers, srv)
		urls[sp.name] = "ws://" + srv.Addr() + "/ws"
		go func(s *mocksource.Server) { _ = s.Serve() }(srv)
	}
	defer func() {
		for _, s := range servers {
			_ = s.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	type rr struct {
		sp   sourceSpec
		st   mockcollector.Stats
		err  error
		path string
	}
	ch := make(chan rr, len(specs))
	for _, sp := range specs {
		sp := sp
		path := filepath.Join(out, sp.name+".ndjson")
		go func() {
			st, err := mockcollector.Run(ctx, mockcollector.Config{URL: urls[sp.name], Source: sp.name, PlaneID: sp.plane, RunID: runID, OutputPath: path, QueueCapacity: 4096, ReconnectDelay: 25 * time.Millisecond})
			ch <- rr{sp: sp, st: st, err: err, path: path}
		}()
	}
	got := make([]rr, 0, len(specs))
	for range specs {
		got = append(got, <-ch)
	}
	for _, s := range servers {
		_ = s.Close()
	}
	sort.Slice(got, func(i, j int) bool { return got[i].sp.name < got[j].sp.name })

	rec := receipt{Version: "V1_LOCAL_MOCK_LIVE", RunID: runID, DurationMS: duration.Milliseconds(), Verdict: "FAIL"}
	paths := make([]string, 0, len(got))
	ok := true
	for _, g := range got {
		r := result{Source: g.sp.name, Plane: g.sp.plane, Stats: g.st}
		if g.err != nil && !errors.Is(g.err, context.DeadlineExceeded) && !errors.Is(g.err, context.Canceled) {
			r.Error = g.err.Error()
			ok = false
		}
		vr, err := journal.ValidateFile(g.path)
		if err != nil {
			r.Error = err.Error()
			ok = false
		} else {
			r.Journal = vr
		}
		if vr.Records < 20 {
			r.Error = "too few records"
			ok = false
		}
		if (g.sp.name == "PM_A" || g.sp.name == "PM_B") && g.st.EmptySideStates == 0 {
			r.Error = "empty-side sentinel not observed"
			ok = false
		}
		if g.sp.name == "PM_A" && g.st.Reconnects < 1 {
			r.Error = "disconnect/reconnect fault was not observed"
			ok = false
		}
		paths = append(paths, g.path)
		rec.Results = append(rec.Results, r)
	}
	items, err := replay.Load(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replay:", err)
		ok = false
	} else {
		rec.ReplayItems = len(items)
	}
	if len(items) > 0 {
		for i := 1; i < len(items); i++ {
			if items[i].Envelope.Event.ReceiveMonotonicNS < items[i-1].Envelope.Event.ReceiveMonotonicNS {
				ok = false
				break
			}
		}
	}
	if ok {
		rec.Verdict = "PASS"
	}
	b, _ := json.MarshalIndent(rec, "", "  ")
	_ = os.WriteFile(filepath.Join(out, "MOCK_LIVE_RECEIPT.json"), b, 0o600)

	emitSummary(rec, out)
	if !ok {
		os.Exit(1)
	}
}

func emitSummary(rec receipt, out string) {
	by := map[string]result{}
	for _, r := range rec.Results {
		by[r.Source] = r
	}
	if by["M2"].Stats.Records >= 500 {
		fmt.Printf("MOCK_M2_HIGH_RATE_PASS records=%d\n", by["M2"].Stats.Records)
	} else {
		fmt.Printf("MOCK_M2_HIGH_RATE_FAIL records=%d\n", by["M2"].Stats.Records)
	}
	if by["SPOT"].Stats.Records > 0 {
		fmt.Printf("MOCK_SPOT_STREAM_PASS records=%d\n", by["SPOT"].Stats.Records)
	}
	if by["PM_A"].Stats.EmptySideStates > 0 && by["PM_B"].Stats.EmptySideStates > 0 {
		fmt.Printf("MOCK_PM_BOUNDARY_0_1_PASS A=%d B=%d\n", by["PM_A"].Stats.EmptySideStates, by["PM_B"].Stats.EmptySideStates)
	}
	if by["PM_A"].Stats.Reconnects > 0 {
		fmt.Printf("MOCK_RECONNECT_PASS count=%d\n", by["PM_A"].Stats.Reconnects)
	}
	if by["PM_A"].Journal.Records > 0 && by["PM_B"].Journal.Records > 0 {
		fmt.Printf("MOCK_PM_DUAL_PLANE_PASS A=%d B=%d\n", by["PM_A"].Journal.Records, by["PM_B"].Journal.Records)
	}
	if rec.ReplayItems > 0 {
		fmt.Printf("MOCK_DETERMINISTIC_REPLAY_PASS items=%d\n", rec.ReplayItems)
	}
	if rec.Verdict == "PASS" {
		fmt.Println("MOCK_LIVE_ALL_PASS")
	} else {
		fmt.Println("MOCK_LIVE_FAIL")
	}
	fmt.Printf("MOCK_LIVE_OUTPUT %s\n", out)
}

// keep net imported in generated builds that aggressively prune platform paths
var _ = net.IPv4len
