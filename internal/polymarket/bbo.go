package polymarket

import (
	"fmt"
	"math/big"
	"strings"
)

// BBOState uses integer ticks at the market tick size. nil means a natural empty side.
type BBOState struct {
	UpBidTicks   *int64 `json:"up_bid_ticks"`
	UpAskTicks   *int64 `json:"up_ask_ticks"`
	DownBidTicks *int64 `json:"down_bid_ticks"`
	DownAskTicks *int64 `json:"down_ask_ticks"`
}

// NormalizeEmbeddedBest converts an embedded best_bid/best_ask value to ticks.
// Polymarket embedded BBO semantics observed in the project:
//
//	best_bid == 0 => natural empty bid
//	best_ask == 1 => natural empty ask
//
// Wrong-side boundary values remain invalid:
//
//	best_bid == 1 => invalid
//	best_ask == 0 => invalid
//
// Executable non-empty prices must be strictly inside (0,1).
func NormalizeEmbeddedBest(raw, side, tickSize string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty %s", side)
	}
	p, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid %s price %q", side, raw)
	}
	zero := big.NewRat(0, 1)
	one := big.NewRat(1, 1)

	switch side {
	case "bid":
		if p.Cmp(zero) == 0 {
			return nil, nil
		}
		if p.Cmp(one) >= 0 {
			return nil, fmt.Errorf("invalid bid boundary %q", raw)
		}
	case "ask":
		if p.Cmp(one) == 0 {
			return nil, nil
		}
		if p.Cmp(zero) <= 0 {
			return nil, fmt.Errorf("invalid ask boundary %q", raw)
		}
	default:
		return nil, fmt.Errorf("unknown side %q", side)
	}

	tick, ok := new(big.Rat).SetString(strings.TrimSpace(tickSize))
	if !ok || tick.Sign() <= 0 {
		return nil, fmt.Errorf("invalid tick size %q", tickSize)
	}
	q := new(big.Rat).Quo(p, tick)
	if !q.IsInt() {
		return nil, fmt.Errorf("price %q not aligned to tick %q", raw, tickSize)
	}
	n := q.Num()
	if !n.IsInt64() {
		return nil, fmt.Errorf("tick count overflow")
	}
	v := n.Int64()
	return &v, nil
}
