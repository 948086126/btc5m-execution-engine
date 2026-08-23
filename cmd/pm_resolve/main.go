package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"btc5m-execution-engine/internal/polymarket"
)

func main() {
	proxy := flag.String("proxy", "", "optional HTTP proxy URL")
	atUnix := flag.Int64("at-unix", 0, "resolve slot containing this unix second; default now")
	flag.Parse()
	now := time.Now().UTC()
	if *atUnix != 0 { now = time.Unix(*atUnix, 0).UTC() }
	m, err := polymarket.ResolveCurrentBTC5M(context.Background(), polymarket.ResolveOptions{Now: now, ProxyURL: *proxy})
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
