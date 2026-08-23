package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"btc5m-execution-engine/internal/livesource"
	"btc5m-execution-engine/internal/polymarket"
)

func main() {
	proxy := flag.String("proxy", "", "optional HTTP proxy URL")
	duration := flag.Duration("duration", 10*time.Second, "capture duration")
	outDir := flag.String("out-dir", "artifacts/v2-pm-smoke", "output directory")
	flag.Parse()
	if *duration <= 0 { fatal("duration must be positive") }
	if err := os.MkdirAll(*outDir, 0o755); err != nil { fatal(err.Error()) }

	m, err := polymarket.ResolveCurrentBTC5M(context.Background(), polymarket.ResolveOptions{ProxyURL: *proxy})
	if err != nil { fatal("resolve current BTC5m: " + err.Error()) }
	metaPath := filepath.Join(*outDir, "market.json")
	if b, err := json.MarshalIndent(m, "", "  "); err != nil { fatal(err.Error()) } else if err := os.WriteFile(metaPath, append(b, '\n'), 0o600); err != nil { fatal(err.Error()) }

	runID := fmt.Sprintf("pm-dual-%d", time.Now().UTC().UnixMilli())
	base := livesource.Config{
		RunID: runID, Source: "PM", ProxyURL: *proxy,
		MarketID: m.ConditionID, MarketSlug: m.Slug,
		UpToken: m.UpToken, DownToken: m.DownToken, TickSize: m.TickSize,
		Duration: *duration,
		URL: "wss://ws-subscriptions-clob.polymarket.com/ws/market",
	}
	cfgA := base; cfgA.PlaneID = "A"; cfgA.OutPath = filepath.Join(*outDir, "pm-a.jsonl")
	cfgB := base; cfgB.PlaneID = "B"; cfgB.OutPath = filepath.Join(*outDir, "pm-b.jsonl")

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, cfg := range []livesource.Config{cfgA, cfgB} {
		wg.Add(1)
		go func(c livesource.Config) {
			defer wg.Done()
			if err := livesource.Run(c); err != nil { errCh <- fmt.Errorf("plane %s: %w", c.PlaneID, err) }
		}(cfg)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh { errs = append(errs, err) }
	if err := errors.Join(errs...); err != nil { fatal(err.Error()) }
	fmt.Printf("PM_DUAL_SMOKE_COMPLETE slug=%s run_id=%s out=%s\n", m.Slug, runID, *outDir)
}

func fatal(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(2) }
