package mockwire

// Message is the deterministic wire schema emitted by V1 loopback mock feeds.
// It is intentionally separate from normalized event.Record so parsing is
// exercised in the collector.
type Message struct {
	Type       string `json:"type"`
	Source     string `json:"source"`
	WireSeq    uint64 `json:"wire_seq"`
	EventMS    int64  `json:"event_ms"`
	Bid        string `json:"bid,omitempty"`
	BidSize    string `json:"bid_size,omitempty"`
	Ask        string `json:"ask,omitempty"`
	AskSize    string `json:"ask_size,omitempty"`
	UpBid      string `json:"up_bid,omitempty"`
	UpAsk      string `json:"up_ask,omitempty"`
	DownBid    string `json:"down_bid,omitempty"`
	DownAsk    string `json:"down_ask,omitempty"`
	TickSize   string `json:"tick_size,omitempty"`
	MarketID   string `json:"market_id,omitempty"`
	MarketSlug string `json:"market_slug,omitempty"`
}
