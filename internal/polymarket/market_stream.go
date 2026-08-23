package polymarket

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type PriceChange struct {
	AssetID string
	BestBid string
	BestAsk string
}

type marketFrame struct {
	EventType    string `json:"event_type"`
	AssetID      string `json:"asset_id"`
	BestBid      string `json:"best_bid"`
	BestAsk      string `json:"best_ask"`
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
	case "book", "last_trade_price", "tick_size_change", "new_market", "market_resolved", "":
		return nil, nil
	default:
		// Unknown event types are ignored rather than treated as malformed BBO.
		return nil, nil
	}
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
