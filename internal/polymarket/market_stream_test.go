package polymarket

import "testing"

func TestParsePriceChangeAndBoundary(t *testing.T) {
	frame := []byte(`{"event_type":"price_change","price_changes":[{"asset_id":"UP","best_bid":"0.99","best_ask":"1"},{"asset_id":"DOWN","best_bid":"0","best_ask":"0.01"}]}`)
	changes, err := ParseEmbeddedBBOChanges(frame)
	if err != nil { t.Fatal(err) }
	if len(changes) != 2 { t.Fatalf("got %d changes", len(changes)) }
	s := NewDualTokenState("UP", "DOWN", "0.01")
	if _, ready, err := s.Apply(changes[0]); err != nil || ready { t.Fatalf("first apply ready=%v err=%v", ready, err) }
	bbo, ready, err := s.Apply(changes[1])
	if err != nil || !ready { t.Fatalf("second apply ready=%v err=%v", ready, err) }
	if bbo.UpBidTicks == nil || *bbo.UpBidTicks != 99 || bbo.UpAskTicks != nil || bbo.DownBidTicks != nil || bbo.DownAskTicks == nil || *bbo.DownAskTicks != 1 { t.Fatalf("unexpected bbo %+v", bbo) }
}

func TestArrayBookFramesBecomeAnchor(t *testing.T) {
	frame := []byte(`[
		{"event_type":"book","asset_id":"UP","bids":[{"price":"0.48","size":"3"},{"price":"0.50","size":"2"}],"asks":[{"price":"0.55","size":"2"},{"price":"0.52","size":"1"}]},
		{"event_type":"book","asset_id":"DOWN","bids":[],"asks":[{"price":"0.49","size":"4"}]}
	]`)
	changes, err := ParseEmbeddedBBOChanges(frame)
	if err != nil { t.Fatal(err) }
	if len(changes) != 2 { t.Fatalf("expected 2 anchor changes, got %d", len(changes)) }
	if changes[0].BestBid != "0.50" || changes[0].BestAsk != "0.52" { t.Fatalf("bad UP best %+v", changes[0]) }
	if changes[1].BestBid != "0" || changes[1].BestAsk != "0.49" { t.Fatalf("bad DOWN best %+v", changes[1]) }
	s := NewDualTokenState("UP", "DOWN", "0.01")
	if _, ready, err := s.Apply(changes[0]); err != nil || ready { t.Fatalf("first ready=%v err=%v", ready, err) }
	bbo, ready, err := s.Apply(changes[1])
	if err != nil || !ready { t.Fatalf("second ready=%v err=%v", ready, err) }
	if bbo.UpBidTicks == nil || *bbo.UpBidTicks != 50 || bbo.UpAskTicks == nil || *bbo.UpAskTicks != 52 || bbo.DownBidTicks != nil || bbo.DownAskTicks == nil || *bbo.DownAskTicks != 49 { t.Fatalf("bad bbo %+v", bbo) }
}
