package event

import "encoding/json"

// Kind is deliberately small in V0. Strategy/order events are not allowed yet.
type Kind string

const (
	KindM2Quote        Kind = "M2_QUOTE"
	KindSpotQuote      Kind = "SPOT_QUOTE"
	KindPMBBOState     Kind = "PM_BBO_STATE"
	KindMarketMeta     Kind = "MARKET_METADATA"
	KindTransport      Kind = "TRANSPORT_STATE"
	KindStateHeartbeat Kind = "STATE_HEARTBEAT"
)

// Record is a normalized source event before journal-owned fields are added.
// Payload is always nested, so it can never overwrite journal sequence/hash fields.
type Record struct {
	SchemaVersion         string          `json:"schema_version"`
	RunID                 string          `json:"run_id"`
	MarketID              string          `json:"market_id,omitempty"`
	MarketSlug            string          `json:"market_slug,omitempty"`
	Source                string          `json:"source"`
	PlaneID               string          `json:"plane_id,omitempty"`
	SourceSessionID       string          `json:"source_session_id"`
	SourceReceiveSequence uint64          `json:"source_receive_sequence"`
	ReceiveWallMS         int64           `json:"receive_wall_ms"`
	ReceiveMonotonicNS    int64           `json:"receive_monotonic_ns"`
	SourceEventMS         *int64          `json:"source_event_ms,omitempty"`
	Kind                  Kind            `json:"event_kind"`
	TransportValidity     string          `json:"transport_validity"`
	FrameSHA256           string          `json:"frame_sha256,omitempty"`
	Payload               json.RawMessage `json:"payload"`
}
