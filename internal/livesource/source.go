package livesource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc5m-execution-engine/internal/asyncwriter"
	"btc5m-execution-engine/internal/binance"
	btclock "btc5m-execution-engine/internal/clock"
	"btc5m-execution-engine/internal/event"
	"btc5m-execution-engine/internal/journal"
	"btc5m-execution-engine/internal/livews"
	"btc5m-execution-engine/internal/polymarket"
)

const schemaVersion = "GATE1A_EVENT_V2"

type Config struct {
	RunID        string
	Source       string
	PlaneID      string
	OutPath      string
	URL          string
	ProxyURL     string
	MarketID     string
	MarketSlug   string
	UpToken      string
	DownToken    string
	TickSize     string
	Duration     time.Duration
	ReconnectMin time.Duration
	ReconnectMax time.Duration
}

type transportPayload struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type quotePayload struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	BidQty float64 `json:"bid_qty"`
	Ask    float64 `json:"ask"`
	AskQty float64 `json:"ask_qty"`
}

type observedFrame struct {
	sequence uint64
	wallMS   int64
	monoNS   int64
	hash     string
	bytes    []byte
}

func Run(cfg Config) error {
	if cfg.RunID == "" || cfg.Source == "" || cfg.OutPath == "" {
		return errors.New("run_id, source and out_path are required")
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 30 * time.Second
	}
	if cfg.ReconnectMin <= 0 {
		cfg.ReconnectMin = 250 * time.Millisecond
	}
	if cfg.ReconnectMax <= 0 {
		cfg.ReconnectMax = 5 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(cfg.OutPath), 0o755); err != nil {
		return err
	}
	jw, err := journal.NewWriter(cfg.OutPath)
	if err != nil {
		return err
	}
	aw := asyncwriter.New(jw, asyncwriter.Options{QueueCapacity: 8192, EnqueueTimeout: 250 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()
	backoff := cfg.ReconnectMin
	var runErr error
	var lastSessionErr error
	var dataRecords uint64
	for session := uint64(1); ; session++ {
		select {
		case <-ctx.Done():
			runErr = nil
			goto done
		default:
		}
		err := runSession(ctx, aw, cfg, session, &dataRecords)
		if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			runErr = nil
			goto done
		}
		lastSessionErr = err
		if aw.Err() != nil {
			runErr = aw.Err()
			goto done
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			runErr = nil
			goto done
		case <-t.C:
		}
		backoff *= 2
		if backoff > cfg.ReconnectMax {
			backoff = cfg.ReconnectMax
		}
	}

done:
	closeErr := aw.Close()
	if dataRecords == 0 {
		if lastSessionErr != nil {
			return errors.Join(fmt.Errorf("no market data records captured; last session error: %w", lastSessionErr), closeErr)
		}
		return errors.Join(errors.New("no market data records captured"), closeErr)
	}
	return errors.Join(runErr, closeErr)
}

func runSession(ctx context.Context, aw *asyncwriter.Writer, cfg Config, sessionNo uint64, dataRecords *uint64) error {
	sessionID := fmt.Sprintf("%s-%s-%d-%d", cfg.RunID, strings.ToLower(cfg.Source), time.Now().UnixMilli(), sessionNo)
	c, err := livews.DialWithProxy(cfg.URL, cfg.ProxyURL, 10*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()

	var rawSeq uint64
	emitTransport := func(state, detail string) error {
		mono, err := btclock.MonotonicNS()
		if err != nil {
			return err
		}
		b, _ := json.Marshal(transportPayload{State: state, Detail: detail})
		return aw.Enqueue(event.Record{SchemaVersion: schemaVersion, RunID: cfg.RunID, MarketID: cfg.MarketID, MarketSlug: cfg.MarketSlug, Source: cfg.Source, PlaneID: cfg.PlaneID, SourceSessionID: sessionID, SourceReceiveSequence: 0, ReceiveWallMS: time.Now().UnixMilli(), ReceiveMonotonicNS: mono, Kind: event.KindTransport, TransportValidity: "VALID", Payload: b})
	}
	observe := func(frame []byte) (observedFrame, error) {
		rawSeq++
		mono, err := btclock.MonotonicNS()
		if err != nil {
			return observedFrame{}, err
		}
		s := sha256.Sum256(frame)
		return observedFrame{sequence: rawSeq, wallMS: time.Now().UnixMilli(), monoNS: mono, hash: hex.EncodeToString(s[:]), bytes: frame}, nil
	}
	emitObserved := func(obs observedFrame, kind event.Kind, sourceEventMS *int64, payload any) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		rec := event.Record{SchemaVersion: schemaVersion, RunID: cfg.RunID, MarketID: cfg.MarketID, MarketSlug: cfg.MarketSlug, Source: cfg.Source, PlaneID: cfg.PlaneID, SourceSessionID: sessionID, SourceReceiveSequence: obs.sequence, ReceiveWallMS: obs.wallMS, ReceiveMonotonicNS: obs.monoNS, SourceEventMS: sourceEventMS, Kind: kind, TransportValidity: "VALID", FrameSHA256: obs.hash, Payload: b}
		if err := aw.Enqueue(rec); err != nil {
			return err
		}
		*dataRecords = *dataRecords + 1
		return nil
	}

	if err := emitTransport("CONNECTED", ""); err != nil {
		return err
	}
	if strings.EqualFold(cfg.Source, "PM") {
		if cfg.UpToken == "" || cfg.DownToken == "" || cfg.TickSize == "" {
			return errors.New("PM source requires up_token, down_token, tick_size")
		}
		sub, _ := json.Marshal(map[string]any{"assets_ids": []string{cfg.UpToken, cfg.DownToken}, "type": "market"})
		if err := c.WriteText(sub); err != nil {
			return err
		}
		return readPM(ctx, c, observe, emitObserved, cfg)
	}
	return readBinance(ctx, c, observe, emitObserved, cfg)
}

func readBinance(ctx context.Context, c *livews.Client, observe func([]byte) (observedFrame, error), emit func(observedFrame, event.Kind, *int64, any) error, cfg Config) error {
	kind := event.KindM2Quote
	if strings.EqualFold(cfg.Source, "SPOT") {
		kind = event.KindSpotQuote
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
		frame, err := c.ReadText(1 << 20)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return fmt.Errorf("read timeout: %w", err)
			}
			return err
		}
		obs, err := observe(frame)
		if err != nil {
			return err
		}
		q, err := binance.ParseBookTicker(frame, "BTCUSDT")
		if err != nil {
			return fmt.Errorf("binance decode after seq=%d: %w", obs.sequence, err)
		}
		var ep *int64
		if q.EventMS > 0 {
			v := q.EventMS
			ep = &v
		}
		if err := emit(obs, kind, ep, quotePayload{Symbol: q.Symbol, Bid: q.Bid, BidQty: q.BidQty, Ask: q.Ask, AskQty: q.AskQty}); err != nil {
			return err
		}
	}
}

func readPM(ctx context.Context, c *livews.Client, observe func([]byte) (observedFrame, error), emit func(observedFrame, event.Kind, *int64, any) error, cfg Config) error {
	state := polymarket.NewDualTokenState(cfg.UpToken, cfg.DownToken, cfg.TickSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
		frame, err := c.ReadText(4 << 20)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return fmt.Errorf("read timeout: %w", err)
			}
			return err
		}
		obs, err := observe(frame)
		if err != nil {
			return err
		}
		changes, err := polymarket.ParseEmbeddedBBOChanges(frame)
		if err != nil {
			return fmt.Errorf("pm decode after seq=%d: %w", obs.sequence, err)
		}
		var last polymarket.BBOState
		ready := false
		for _, ch := range changes {
			bbo, ok, err := state.Apply(ch)
			if err != nil {
				return fmt.Errorf("pm state after seq=%d: %w", obs.sequence, err)
			}
			if ok {
				last, ready = bbo, true
			}
		}
		if ready {
			if err := emit(obs, event.KindPMBBOState, nil, last); err != nil {
				return err
			}
		}
	}
}
