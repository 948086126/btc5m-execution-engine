package binance

import "testing"

func TestParseFuturesBookTickerWithEventTypeAndEventTime(t *testing.T) {
	frame := []byte(`{"e":"bookTicker","E":1787490000123,"u":123,"s":"BTCUSDT","b":"64000.10","B":"1.25","a":"64000.20","A":"2.50","T":1787490000122}`)
	q, err := ParseBookTicker(frame, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if q.Bid != 64000.10 || q.Ask != 64000.20 || q.EventMS != 1787490000123 {
		t.Fatalf("unexpected %+v", q)
	}
}

func TestParseSpotBookTickerWithoutEventTime(t *testing.T) {
	frame := []byte(`{"u":400900217,"s":"BTCUSDT","b":"64000.10","B":"1.25","a":"64000.20","A":"2.50"}`)
	q, err := ParseBookTicker(frame, "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if q.EventMS != 0 || q.Bid != 64000.10 || q.Ask != 64000.20 {
		t.Fatalf("unexpected %+v", q)
	}
}

func TestRejectWrongEventType(t *testing.T) {
	frame := []byte(`{"e":"trade","E":1787490000123,"s":"BTCUSDT","b":"64000.10","B":"1.25","a":"64000.20","A":"2.50"}`)
	if _, err := ParseBookTicker(frame, "BTCUSDT"); err == nil {
		t.Fatal("expected wrong event type rejection")
	}
}

func TestRejectCrossedBook(t *testing.T) {
	frame := []byte(`{"E":1,"s":"BTCUSDT","b":"101","B":"1","a":"100","A":"1"}`)
	if _, err := ParseBookTicker(frame, "BTCUSDT"); err == nil {
		t.Fatal("expected crossed book rejection")
	}
}
