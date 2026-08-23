package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	replaypkg "btc5m-execution-engine/internal/replay"
)

type merged struct {
	GlobalSequence uint64 `json:"global_sequence"`
	SourceJournal  string `json:"source_journal"`
	SourceSequence uint64 `json:"source_sequence"`
	Event          any    `json:"event"`
}

func main() {
	journals := flag.String("journals", "", "comma-separated journal paths")
	flag.Parse()
	if strings.TrimSpace(*journals) == "" {
		fmt.Fprintln(os.Stderr, "--journals is required")
		os.Exit(2)
	}
	items, err := replaypkg.Load(strings.Split(*journals, ","))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	for i, item := range items {
		m := merged{GlobalSequence: uint64(i + 1), SourceJournal: item.JournalPath, SourceSequence: item.Envelope.Sequence, Event: item.Envelope.Event}
		if err := enc.Encode(m); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
