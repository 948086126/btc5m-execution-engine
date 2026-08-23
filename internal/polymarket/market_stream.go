package polymarket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
)

type PriceChange struct {
	AssetID string
	BestBid string
	BestAsk string
}

type level struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type marketFrame struct {
	EventType    string `json:"event_type"`
	AssetID      string `json:"asset_id"`
	BestBid      string `json:"best_bid"`
	BestAsk      string `json:"best_ask"`
	Bids         []level `json:"bids"`
	Asks         []level `json:"asks"`
	PriceChanges []struct {
		AssetID string `json:"asset_id"`
		BestBid string `json:"best_bid"`
		BestAsk string `json:"best_ask"`
	} `json:"price_changes"`
}

func ParseEmbeddedBBOChanges(frame []byte) ([]PriceChange, error) {
	frame = bytes.TrimSpace(frame)
	if len(frame) == 0 {
		return nil, nil
	}
	if frame[0] == '[' {
		var raws []json.RawMessage
		if err := json.Unmarshal(frame, &raws); err != nil {
			return nil, err
		}
		var out []PriceChange
		for _, raw := range raws {
			changes, err := parseOneMarketFrame(raw)
			if err != nil {
				return nil, err
			}
			out = append(out, changes...)
		}
		return out, nil
	}
	return parseOneMarketFrame(frame)
}

func parseOneMarketFrame(frame []byte) ([]PriceChange, error) {
	var m marketFrame
	if err := json.Unmarshal(frame, &m); err != nil {
		return nil, err
	}
	switch m.EventType {
	case "price_change":
		out := make([]PriceChange, 0, len(m.PriceChanges))
		for _, p := range m.PriceChanges {
			if p.AssetID == "" {
				return nil, fmt.Errorf("price_change missing asset_id")
			}
			if p.BestBid == "" || p.BestAsk == "" {
				continue
			}
			out = append(out, PriceChange{AssetID: p.AssetID, BestBid: p.BestBid, BestAsk: p.BestAsk})
		}
		return out, nil
	case "best_bid_ask":
		if m.AssetID == "" || m.BestBid == "" || m.BestAsk == "" {
			return nil, fmt.Errorf("best_bid_ask missing fields")
		}
		return []PriceChange{{AssetID: m.AssetID, BestBid: m.BestBid, BestAsk: m.BestAsk}}, nil
	case "book":
		if m.AssetID == "" {
			return nil, fmt.Errorf("book missing asset_id")
		}
		bid, err := bestLevelPrice(m.Bids, true)
		if err != nil { return nil, fmt.Errorf("book bid: %w", err) }
		ask, err := bestLevelPrice(m.Asks, false)
		if err != nil { return nil, fmt.Errorf("book ask: %w", err) }
		// Reuse the frozen embedded-BBO boundary semantics downstream:
		// no bid => 0, no ask => 1.
		if bid == "" { bid = "0" }
		if ask == "" { ask = "1" }
		return []PriceChange{{AssetID: m.AssetID, BestBid: bid, BestAsk: ask}}, nil
	case "last_trade_price", "tick_size_change", "new_market", "market_resolved", "":
		return nil, nil
	default:
		// Unknown event types are ignored rather than treated as malformed BBO.
		return nil, nil
	}
}

func bestLevelPrice(levels []level, wantMax bool) (string, error) {
	var best *big.Rat
	bestRaw := ""
	for _, l := range levels {
		p, ok := new(big.Rat).SetString(l.Price)
		if !ok { return "", fmt.Errorf("invalid price %q", l.Price) }
		if p.Sign() <= 0 || p.Cmp(big.NewRat(1,1)) >= 0 { return "", fmt.Errorf("price outside (0,1): %q", l.Price) }
		if best == nil || (wantMax && p.Cmp(best) > 0) || (!wantMax && p.Cmp(best) < 0) {
			best = p
			bestRaw = l.Price
		}
	}
	return bestRaw, nil
}

type DualTokenState struct {
	UpToken   string
	DownToken string
	TickSize  string
	upBid     *int64
	upAsk     *int64
	downBid   *int64
	downAsk   *int64
	haveUp    bool
	haveDown  bool
}

func NewDualTokenState(upToken, downToken, tickSize string) *DualTokenState {
	return &DualTokenState{UpToken: upToken, DownToken: downToken, TickSize: tickSize}
}

func (s *DualTokenState) Apply(c PriceChange) (BBOState, bool, error) {
	bid, err := NormalizeEmbeddedBest(c.BestBid, "bid", s.TickSize)
	if err != nil {
		return BBOState{}, false, err
	}
	ask, err := NormalizeEmbeddedBest(c.BestAsk, "ask", s.TickSize)
	if err != nil {
		return BBOState{}, false, err
	}
	switch c.AssetID {
	case s.UpToken:
		s.upBid, s.upAsk, s.haveUp = bid, ask, true
	case s.DownToken:
		s.downBid, s.downAsk, s.haveDown = bid, ask, true
	default:
		return BBOState{}, false, fmt.Errorf("unknown token %s", c.AssetID)
	}
	if !s.haveUp || !s.haveDown {
		return BBOState{}, false, nil
	}
	return BBOState{UpBidTicks: s.upBid, UpAskTicks: s.upAsk, DownBidTicks: s.downBid, DownAskTicks: s.downAsk}, true, nil
}
