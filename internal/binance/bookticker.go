package binance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type BookTicker struct {
	EventMS int64   `json:"event_ms"`
	Symbol  string  `json:"symbol"`
	Bid     float64 `json:"bid"`
	BidQty  float64 `json:"bid_qty"`
	Ask     float64 `json:"ask"`
	AskQty  float64 `json:"ask_qty"`
}

type wireBookTicker struct {
	// IMPORTANT: keep both lower-case `e` and upper-case `E` explicitly.
	// encoding/json matches field names/tags case-insensitively as a fallback;
	// without EventType, a real Futures `"e":"bookTicker"` field can be
	// incorrectly matched to `json:"E"` and fail unmarshalling into int64.
	EventType       string `json:"e"`
	EventTime       int64  `json:"E"`
	UpdateID        uint64 `json:"u"`
	TransactionTime int64  `json:"T"`
	Symbol          string `json:"s"`
	Bid             string `json:"b"`
	BidQty          string `json:"B"`
	Ask             string `json:"a"`
	AskQty          string `json:"A"`
}

func ParseBookTicker(frame []byte, wantSymbol string) (BookTicker, error) {
	var w wireBookTicker
	if err := json.Unmarshal(frame, &w); err != nil {
		return BookTicker{}, err
	}
	if w.EventType != "" && w.EventType != "bookTicker" {
		return BookTicker{}, fmt.Errorf("unexpected event type %q", w.EventType)
	}
	if !strings.EqualFold(w.Symbol, wantSymbol) {
		return BookTicker{}, fmt.Errorf("unexpected symbol %q", w.Symbol)
	}
	parse := func(name, s string) (float64, error) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		if v < 0 {
			return 0, fmt.Errorf("%s negative", name)
		}
		return v, nil
	}
	bid, err := parse("bid", w.Bid)
	if err != nil {
		return BookTicker{}, err
	}
	ask, err := parse("ask", w.Ask)
	if err != nil {
		return BookTicker{}, err
	}
	bq, err := parse("bid_qty", w.BidQty)
	if err != nil {
		return BookTicker{}, err
	}
	aq, err := parse("ask_qty", w.AskQty)
	if err != nil {
		return BookTicker{}, err
	}
	if bid <= 0 || ask <= 0 || bid > ask {
		return BookTicker{}, fmt.Errorf("invalid book bid=%v ask=%v", bid, ask)
	}
	return BookTicker{
		EventMS: w.EventTime,
		Symbol:  strings.ToUpper(w.Symbol),
		Bid:     bid,
		BidQty:  bq,
		Ask:     ask,
		AskQty:  aq,
	}, nil
}
