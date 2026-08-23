package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"btc5m-execution-engine/internal/livesource"
)

func main() {
	var cfg livesource.Config
	var duration time.Duration
	flag.StringVar(&cfg.Source, "source", "", "m2, spot, or pm")
	flag.StringVar(&cfg.PlaneID, "plane", "", "PM plane identifier, e.g. A or B")
	flag.StringVar(&cfg.RunID, "run-id", "", "run identifier")
	flag.StringVar(&cfg.OutPath, "out", "", "journal output path")
	flag.StringVar(&cfg.URL, "url", "", "override websocket URL")
	flag.StringVar(&cfg.ProxyURL, "proxy", "", "optional HTTP CONNECT proxy, e.g. http://127.0.0.1:7897")
	flag.StringVar(&cfg.MarketID, "market-id", "", "Polymarket market id")
	flag.StringVar(&cfg.MarketSlug, "market-slug", "", "Polymarket market slug")
	flag.StringVar(&cfg.UpToken, "up-token", "", "Polymarket UP token id")
	flag.StringVar(&cfg.DownToken, "down-token", "", "Polymarket DOWN token id")
	flag.StringVar(&cfg.TickSize, "tick-size", "0.01", "Polymarket tick size")
	flag.DurationVar(&duration, "duration", 30*time.Second, "run duration")
	flag.Parse()
	cfg.Source = strings.ToUpper(strings.TrimSpace(cfg.Source))
	cfg.Duration = duration
	if cfg.URL == "" {
		switch cfg.Source {
		case "M2":
			cfg.URL = "wss://fstream.binance.com/ws/btcusdt@bookTicker"
		case "SPOT":
			cfg.URL = "wss://stream.binance.com:9443/ws/btcusdt@bookTicker"
		case "PM":
			cfg.URL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
		default:
			fatal("--source must be m2, spot, or pm")
		}
	}
	if cfg.RunID == "" || cfg.OutPath == "" {
		fatal("--run-id and --out are required")
	}
	if cfg.Source == "PM" && cfg.PlaneID == "" {
		fatal("PM requires --plane A or B")
	}
	if err := livesource.Run(cfg); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("LIVE_SOURCE_COMPLETE source=%s plane=%s out=%s\n", cfg.Source, cfg.PlaneID, cfg.OutPath)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}
