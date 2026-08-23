package polymarket

import "testing"

func TestParsePriceChangeAndBoundary(t *testing.T){
	frame:=[]byte(`{"event_type":"price_change","price_changes":[{"asset_id":"UP","best_bid":"0.99","best_ask":"1"},{"asset_id":"DOWN","best_bid":"0","best_ask":"0.01"}]}`)
	changes,err:=ParseEmbeddedBBOChanges(frame); if err!=nil{t.Fatal(err)}
	if len(changes)!=2{t.Fatalf("got %d changes",len(changes))}
	s:=NewDualTokenState("UP","DOWN","0.01")
	if _,ready,err:=s.Apply(changes[0]);err!=nil||ready{t.Fatalf("first apply ready=%v err=%v",ready,err)}
	bbo,ready,err:=s.Apply(changes[1]);if err!=nil||!ready{t.Fatalf("second apply ready=%v err=%v",ready,err)}
	if bbo.UpBidTicks==nil || *bbo.UpBidTicks!=99 || bbo.UpAskTicks!=nil || bbo.DownBidTicks!=nil || bbo.DownAskTicks==nil || *bbo.DownAskTicks!=1 { t.Fatalf("unexpected bbo %+v",bbo) }
}

func TestArrayBookFramesIgnored(t *testing.T){
	frame:=[]byte(`[{"event_type":"book","asset_id":"UP"},{"event_type":"book","asset_id":"DOWN"}]`)
	changes,err:=ParseEmbeddedBBOChanges(frame); if err!=nil{t.Fatal(err)}
	if len(changes)!=0{t.Fatalf("expected no embedded bbo, got %d",len(changes))}
}
