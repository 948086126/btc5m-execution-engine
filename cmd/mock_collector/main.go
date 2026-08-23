package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"btc5m-execution-engine/internal/mockcollector"
)

func main() {
	var cfg mockcollector.Config
	var duration time.Duration
	flag.StringVar(&cfg.URL, "url", "ws://127.0.0.1:18081/ws", "loopback mock websocket URL")
	flag.StringVar(&cfg.Source, "source", "M2", "source name")
	flag.StringVar(&cfg.PlaneID, "plane", "", "plane id")
	flag.StringVar(&cfg.RunID, "run-id", "mock-debug", "run id")
	flag.StringVar(&cfg.OutputPath, "out", "mock.ndjson", "journal path")
	flag.IntVar(&cfg.QueueCapacity, "queue", 2048, "writer queue capacity")
	flag.DurationVar(&duration, "duration", 5*time.Second, "collection duration")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	stats, err := mockcollector.Run(ctx, cfg)
	b, _ := json.Marshal(stats)
	fmt.Printf("MOCK_COLLECTOR_STATS %s\n", b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
