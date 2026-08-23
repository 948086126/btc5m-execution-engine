package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"btc5m-execution-engine/internal/mocksource"
)

func main() {
	var cfg mocksource.Config
	var delayMS, disconnectMS int
	flag.StringVar(&cfg.Addr, "addr", "127.0.0.1:18081", "loopback listen address")
	flag.StringVar(&cfg.Source, "source", "M2", "M2|SPOT|PM_A|PM_B")
	flag.IntVar(&cfg.RateHz, "rate-hz", 100, "mock frame rate")
	flag.IntVar(&delayMS, "send-delay-ms", 0, "per-frame mock delivery delay")
	flag.IntVar(&disconnectMS, "disconnect-after-ms", 0, "abruptly disconnect the first client once")
	flag.Parse()
	cfg.SendDelay = time.Duration(delayMS) * time.Millisecond
	cfg.DisconnectAfter = time.Duration(disconnectMS) * time.Millisecond

	srv := mocksource.New(cfg)
	if err := srv.Listen(); err != nil {
		panic(err)
	}
	fmt.Printf("MOCK_SOURCE_READY source=%s addr=%s\n", cfg.Source, srv.Addr())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; _ = srv.Close() }()
	if err := srv.Serve(); err != nil && !isClosed(err) {
		panic(err)
	}
}

func isClosed(err error) bool {
	return err != nil && (err.Error() == "use of closed network connection" || contains(err.Error(), "closed network connection"))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
