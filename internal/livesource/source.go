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
	RunID       string
	Source      string
	PlaneID     string
	OutPath     string
	URL         string
	MarketID    string
	MarketSlug  string
	UpToken     string
	DownToken   string
	TickSize    string
	Duration    time.Duration
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

func Run(cfg Config) error {
	if cfg.RunID == "" || cfg.Source == "" || cfg.OutPath == "" { return errors.New("run_id, source and out_path are required") }
	if cfg.Duration <= 0 { cfg.Duration = 30 * time.Second }
	if cfg.ReconnectMin <= 0 { cfg.ReconnectMin = 250 * time.Millisecond }
	if cfg.ReconnectMax <= 0 { cfg.ReconnectMax = 5 * time.Second }
	if err := os.MkdirAll(filepath.Dir(cfg.OutPath), 0o755); err != nil { return err }
	jw, err := journal.NewWriter(cfg.OutPath); if err != nil { return err }
	aw := asyncwriter.New(jw, asyncwriter.Options{QueueCapacity: 8192, EnqueueTimeout: 250 * time.Millisecond})
	defer aw.Close()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration); defer cancel()
	backoff := cfg.ReconnectMin
	for session := uint64(1); ; session++ {
		select { case <-ctx.Done(): return errors.Join(aw.Err(), nil); default: }
		err := runSession(ctx, aw, cfg, session)
		if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) { return errors.Join(aw.Err(), nil) }
		if aw.Err() != nil { return aw.Err() }
		t := time.NewTimer(backoff)
		select { case <-ctx.Done(): t.Stop(); return nil; case <-t.C: }
		backoff *= 2; if backoff > cfg.ReconnectMax { backoff = cfg.ReconnectMax }
	}
}

func runSession(ctx context.Context, aw *asyncwriter.Writer, cfg Config, sessionNo uint64) error {
	sessionID := fmt.Sprintf("%s-%s-%d-%d", cfg.RunID, strings.ToLower(cfg.Source), time.Now().UnixMilli(), sessionNo)
	c, err := livews.Dial(cfg.URL, 10*time.Second)
	if err != nil { return err }
	defer c.Close()
	var seq uint64
	emit := func(kind event.Kind, sourceEventMS *int64, frame []byte, payload any) error {
		seq++
		mono, err := btclock.MonotonicNS(); if err != nil { return err }
		b, err := json.Marshal(payload); if err != nil { return err }
		h := ""; if frame != nil { s:=sha256.Sum256(frame); h=hex.EncodeToString(s[:]) }
		rec := event.Record{SchemaVersion:schemaVersion,RunID:cfg.RunID,MarketID:cfg.MarketID,MarketSlug:cfg.MarketSlug,Source:cfg.Source,PlaneID:cfg.PlaneID,SourceSessionID:sessionID,SourceReceiveSequence:seq,ReceiveWallMS:time.Now().UnixMilli(),ReceiveMonotonicNS:mono,SourceEventMS:sourceEventMS,Kind:kind,TransportValidity:"VALID",FrameSHA256:h,Payload:b}
		return aw.Enqueue(rec)
	}
	if err := emit(event.KindTransport,nil,nil,transportPayload{State:"CONNECTED"}); err != nil { return err }
	if strings.EqualFold(cfg.Source,"PM") {
		if cfg.UpToken=="" || cfg.DownToken=="" || cfg.TickSize=="" { return errors.New("PM source requires up_token, down_token, tick_size") }
		sub,_:=json.Marshal(map[string]any{"assets_ids":[]string{cfg.UpToken,cfg.DownToken},"type":"market"})
		if err:=c.WriteText(sub);err!=nil{return err}
		return readPM(ctx,c,emit,cfg)
	}
	return readBinance(ctx,c,emit,cfg)
}

func readBinance(ctx context.Context, c *livews.Client, emit func(event.Kind,*int64,[]byte,any)error, cfg Config) error {
	kind:=event.KindM2Quote; if strings.EqualFold(cfg.Source,"SPOT") { kind=event.KindSpotQuote }
	for {
		select { case <-ctx.Done(): return ctx.Err(); default: }
		_ = c.SetReadDeadline(time.Now().Add(20*time.Second))
		frame,err:=c.ReadText(1<<20); if err!=nil { if ne,ok:=err.(net.Error);ok&&ne.Timeout(){return fmt.Errorf("read timeout: %w",err)}; return err }
		q,err:=binance.ParseBookTicker(frame,"BTCUSDT"); if err!=nil{return fmt.Errorf("binance decode: %w",err)}
		ep:=q.EventMS
		if err:=emit(kind,&ep,frame,quotePayload{Symbol:q.Symbol,Bid:q.Bid,BidQty:q.BidQty,Ask:q.Ask,AskQty:q.AskQty});err!=nil{return err}
	}
}

func readPM(ctx context.Context, c *livews.Client, emit func(event.Kind,*int64,[]byte,any)error, cfg Config) error {
	state:=polymarket.NewDualTokenState(cfg.UpToken,cfg.DownToken,cfg.TickSize)
	for {
		select { case <-ctx.Done(): return ctx.Err(); default: }
		_ = c.SetReadDeadline(time.Now().Add(20*time.Second))
		frame,err:=c.ReadText(4<<20); if err!=nil { if ne,ok:=err.(net.Error);ok&&ne.Timeout(){return fmt.Errorf("read timeout: %w",err)}; return err }
		changes,err:=polymarket.ParseEmbeddedBBOChanges(frame); if err!=nil{return fmt.Errorf("pm decode: %w",err)}
		for _,ch:=range changes {
			bbo,ready,err:=state.Apply(ch); if err!=nil{return fmt.Errorf("pm state: %w",err)}
			if ready { if err:=emit(event.KindPMBBOState,nil,frame,bbo);err!=nil{return err} }
		}
	}
}
