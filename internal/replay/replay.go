package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"btc5m-execution-engine/internal/journal"
)

type Item struct {
	JournalPath string
	Envelope    journal.Envelope
}

func Load(paths []string) ([]Item, error) {
	var items []Item
	for _, path := range paths {
		if _, err := journal.ValidateFile(path); err != nil {
			return nil, fmt.Errorf("validate %s: %w", path, err)
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for s.Scan() {
			var env journal.Envelope
			if err := json.Unmarshal(s.Bytes(), &env); err != nil {
				f.Close()
				return nil, err
			}
			items = append(items, Item{JournalPath: path, Envelope: env})
		}
		if err := s.Err(); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Envelope.Event, items[j].Envelope.Event
		if a.ReceiveMonotonicNS != b.ReceiveMonotonicNS {
			return a.ReceiveMonotonicNS < b.ReceiveMonotonicNS
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.SourceSessionID != b.SourceSessionID {
			return a.SourceSessionID < b.SourceSessionID
		}
		return a.SourceReceiveSequence < b.SourceReceiveSequence
	})
	return items, nil
}
