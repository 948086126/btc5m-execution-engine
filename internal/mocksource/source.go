package mocksource

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"btc5m-execution-engine/internal/mockwire"
	"btc5m-execution-engine/internal/mockws"
)

type Config struct {
	Addr            string
	Source          string
	RateHz          int
	SendDelay       time.Duration
	DisconnectAfter time.Duration // disconnect first client once after this elapsed duration
	MarketID        string
	MarketSlug      string
}

type Server struct {
	cfg          Config
	listener     net.Listener
	disconnected atomic.Bool
	started      time.Time
	wireSeq      atomic.Uint64
}

func New(cfg Config) *Server {
	if cfg.RateHz <= 0 {
		cfg.RateHz = 100
	}
	if cfg.MarketID == "" {
		cfg.MarketID = "mock-market-1"
	}
	if cfg.MarketSlug == "" {
		cfg.MarketSlug = "btc-updown-5m-mock"
	}
	return &Server{cfg: cfg}
}

func (s *Server) Listen() error {
	host, _, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("V1 mock source refuses non-loopback bind %q", host)
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.started = time.Now()
	return nil
}

func (s *Server) Addr() string { return s.listener.Addr().String() }
func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) Serve() error {
	if s.listener == nil {
		return fmt.Errorf("not listening")
	}
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	rw, err := mockws.Upgrade(conn)
	if err != nil {
		return
	}
	interval := time.Second / time.Duration(s.cfg.RateHz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if s.cfg.DisconnectAfter > 0 && !s.disconnected.Load() && time.Since(s.started) >= s.cfg.DisconnectAfter {
			if s.disconnected.CompareAndSwap(false, true) {
				// abrupt close is intentional transport fault.
				return
			}
		}
		seq := s.wireSeq.Add(1)
		msg := s.message(seq)
		b, _ := json.Marshal(msg)
		if s.cfg.SendDelay > 0 {
			time.Sleep(s.cfg.SendDelay)
		}
		if err := mockws.WriteTextServer(rw.Writer, b); err != nil {
			return
		}
		if err := rw.Flush(); err != nil {
			return
		}
	}
}

func (s *Server) message(seq uint64) mockwire.Message {
	now := time.Now().UnixMilli()
	switch s.cfg.Source {
	case "M2":
		base := 60000.0 + float64((seq/10)%20)*0.5
		return mockwire.Message{Type: "quote", Source: "M2", WireSeq: seq, EventMS: now, Bid: fmt.Sprintf("%.2f", base), Ask: fmt.Sprintf("%.2f", base+0.10), BidSize: "3.5", AskSize: "2.7", MarketID: s.cfg.MarketID, MarketSlug: s.cfg.MarketSlug}
	case "SPOT":
		base := 59999.8 + float64((seq/12)%20)*0.45
		return mockwire.Message{Type: "quote", Source: "SPOT", WireSeq: seq, EventMS: now, Bid: fmt.Sprintf("%.2f", base), Ask: fmt.Sprintf("%.2f", base+0.10), BidSize: "4.1", AskSize: "3.2", MarketID: s.cfg.MarketID, MarketSlug: s.cfg.MarketSlug}
	case "PM_A", "PM_B":
		// Emit legal empty-side 0/1 sentinel values periodically.
		upBid, upAsk, downBid, downAsk := "0.49", "0.51", "0.49", "0.51"
		phase := seq % 40
		if phase >= 10 && phase < 20 {
			upBid, upAsk, downBid, downAsk = "0.50", "0.52", "0.48", "0.50"
		}
		if phase >= 20 && phase < 30 {
			upBid, upAsk, downBid, downAsk = "0.51", "0.53", "0.47", "0.49"
		}
		if phase == 30 {
			upBid, upAsk, downBid, downAsk = "0.99", "1", "0", "0.01"
		}
		return mockwire.Message{Type: "bbo", Source: s.cfg.Source, WireSeq: seq, EventMS: now, UpBid: upBid, UpAsk: upAsk, DownBid: downBid, DownAsk: downAsk, TickSize: "0.01", MarketID: s.cfg.MarketID, MarketSlug: s.cfg.MarketSlug}
	default:
		return mockwire.Message{Type: "heartbeat", Source: s.cfg.Source, WireSeq: seq, EventMS: now}
	}
}

func ParsePort(addr string) int {
	_, p, _ := net.SplitHostPort(addr)
	v, _ := strconv.Atoi(p)
	return v
}
