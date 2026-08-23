package mockcollector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"btc5m-execution-engine/internal/asyncwriter"
	"btc5m-execution-engine/internal/clock"
	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
	"btc5m-execution-engine/internal/mockwire"
	"btc5m-execution-engine/internal/mockws"
	"btc5m-execution-engine/internal/polymarket"
)

type Config struct {
	URL            string
	Source         string
	PlaneID        string
	RunID          string
	OutputPath     string
	QueueCapacity  int
	WriteDelay     time.Duration
	ReconnectDelay time.Duration
}

type Stats struct {
	Records           uint64
	Reconnects        uint64
	DecodeErrors      uint64
	BackpressureTrips uint64
	EmptySideStates   uint64
}

func Run(ctx context.Context, cfg Config) (Stats, error) {
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = 50 * time.Millisecond
	}
	jw, err := journal.NewWriter(cfg.OutputPath)
	if err != nil {
		return Stats{}, err
	}
	aw := asyncwriter.New(jw, asyncwriter.Options{QueueCapacity: cfg.QueueCapacity, EnqueueTimeout: 100 * time.Millisecond, ArtificialWriteDelay: cfg.WriteDelay})
	var stats Stats

	sessionNum := 0
	for {
		select {
		case <-ctx.Done():
			return stats, aw.Close()
		default:
		}
		sessionNum++
		sessionID := fmt.Sprintf("%s-session-%d", cfg.Source, sessionNum)
		conn, br, err := mockws.Dial(cfg.URL, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return stats, aw.Close()
			}
			time.Sleep(cfg.ReconnectDelay)
			continue
		}
		seq := uint64(0)
		_ = enqueueTransport(aw, cfg, sessionID, &seq, "CONNECTED", "VALID")
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			frame, err := mockws.ReadTextClient(br, 2*1024*1024)
			if err != nil {
				_ = conn.Close()
				if errors.Is(err, io.EOF) || ctx.Err() == nil {
					atomic.AddUint64(&stats.Reconnects, 1)
					_ = enqueueTransport(aw, cfg, sessionID, &seq, "DISCONNECTED", "INVALID")
				}
				break
			}
			// Timestamp before parse, by contract.
			wall := time.Now().UnixMilli()
			mono, clockErr := clock.MonotonicNS()
			if clockErr != nil {
				_ = conn.Close()
				return stats, clockErr
			}
			seq++
			rec, empty, err := normalize(cfg, sessionID, seq, wall, mono, frame)
			if err != nil {
				stats.DecodeErrors++
				_ = enqueueTransport(aw, cfg, sessionID, &seq, "DECODE_ERROR", "INVALID")
				_ = conn.Close()
				return stats, err
			}
			if empty {
				stats.EmptySideStates++
			}
			if err := aw.Enqueue(rec); err != nil {
				if errors.Is(err, asyncwriter.ErrBackpressure) {
					stats.BackpressureTrips++
				}
				_ = conn.Close()
				return stats, err
			}
			stats.Records++
			if ctx.Err() != nil {
				_ = conn.Close()
				return stats, aw.Close()
			}
		}
		select {
		case <-ctx.Done():
			return stats, aw.Close()
		case <-time.After(cfg.ReconnectDelay):
		}
	}
}

func enqueueTransport(aw *asyncwriter.Writer, cfg Config, session string, seq *uint64, state, validity string) error {
	(*seq)++
	mono, err := clock.MonotonicNS()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"state": state})
	return aw.Enqueue(event.Record{SchemaVersion: "GATE1A_EVENT_V1", RunID: cfg.RunID, Source: cfg.Source, PlaneID: cfg.PlaneID, SourceSessionID: session, SourceReceiveSequence: *seq, ReceiveWallMS: time.Now().UnixMilli(), ReceiveMonotonicNS: mono, Kind: event.KindTransport, TransportValidity: validity, Payload: payload})
}

func normalize(cfg Config, session string, seq uint64, wall, mono int64, frame []byte) (event.Record, bool, error) {
	var m mockwire.Message
	if err := json.Unmarshal(frame, &m); err != nil {
		return event.Record{}, false, err
	}
	if m.Source != cfg.Source {
		return event.Record{}, false, fmt.Errorf("source mismatch got=%s want=%s", m.Source, cfg.Source)
	}
	h := sha256.Sum256(frame)
	srcEvent := m.EventMS
	base := event.Record{SchemaVersion: "GATE1A_EVENT_V1", RunID: cfg.RunID, MarketID: m.MarketID, MarketSlug: m.MarketSlug, Source: cfg.Source, PlaneID: cfg.PlaneID, SourceSessionID: session, SourceReceiveSequence: seq, ReceiveWallMS: wall, ReceiveMonotonicNS: mono, SourceEventMS: &srcEvent, TransportValidity: "VALID", FrameSHA256: hex.EncodeToString(h[:])}
	switch m.Type {
	case "quote":
		bid, err := strconv.ParseFloat(m.Bid, 64)
		if err != nil || math.IsNaN(bid) || math.IsInf(bid, 0) {
			return event.Record{}, false, fmt.Errorf("invalid bid %q", m.Bid)
		}
		ask, err := strconv.ParseFloat(m.Ask, 64)
		if err != nil || math.IsNaN(ask) || math.IsInf(ask, 0) {
			return event.Record{}, false, fmt.Errorf("invalid ask %q", m.Ask)
		}
		if bid > ask {
			return event.Record{}, false, fmt.Errorf("crossed quote bid=%s ask=%s", m.Bid, m.Ask)
		}
		bs, err := strconv.ParseFloat(m.BidSize, 64)
		if err != nil || math.IsNaN(bs) || math.IsInf(bs, 0) || bs < 0 {
			return event.Record{}, false, fmt.Errorf("invalid bid_size %q", m.BidSize)
		}
		as, err := strconv.ParseFloat(m.AskSize, 64)
		if err != nil || math.IsNaN(as) || math.IsInf(as, 0) || as < 0 {
			return event.Record{}, false, fmt.Errorf("invalid ask_size %q", m.AskSize)
		}
		base.Kind = event.KindM2Quote
		if cfg.Source == "SPOT" {
			base.Kind = event.KindSpotQuote
		}
		payload, _ := json.Marshal(map[string]string{"bid": m.Bid, "bid_size": m.BidSize, "ask": m.Ask, "ask_size": m.AskSize})
		base.Payload = payload
		return base, false, nil
	case "bbo":
		ub, err := polymarket.NormalizeEmbeddedBest(m.UpBid, "bid", m.TickSize)
		if err != nil {
			return event.Record{}, false, err
		}
		ua, err := polymarket.NormalizeEmbeddedBest(m.UpAsk, "ask", m.TickSize)
		if err != nil {
			return event.Record{}, false, err
		}
		db, err := polymarket.NormalizeEmbeddedBest(m.DownBid, "bid", m.TickSize)
		if err != nil {
			return event.Record{}, false, err
		}
		da, err := polymarket.NormalizeEmbeddedBest(m.DownAsk, "ask", m.TickSize)
		if err != nil {
			return event.Record{}, false, err
		}
		state := polymarket.BBOState{UpBidTicks: ub, UpAskTicks: ua, DownBidTicks: db, DownAskTicks: da}
		payload, _ := json.Marshal(state)
		base.Kind = event.KindPMBBOState
		base.Payload = payload
		return base, ub == nil || ua == nil || db == nil || da == nil, nil
	default:
		return event.Record{}, false, fmt.Errorf("unsupported mock type %q", m.Type)
	}
}
