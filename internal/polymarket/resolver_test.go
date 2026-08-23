package polymarket

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCurrentBTC5MSlug(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 53, 28, 0, time.UTC)
	if got, want := CurrentBTC5MSlug(now), "btc-updown-5m-1787500200"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestResolveCurrentBTC5M(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 53, 28, 0, time.UTC)
	slug := CurrentBTC5MSlug(now)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets/slug/"+slug {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"123","conditionId":"0xabc","slug":%q,"startDate":"2026-08-23T15:50:00Z","eventStartTime":"2026-08-23T15:50:00Z","endDate":"2026-08-23T15:55:00Z","active":true,"closed":false,"acceptingOrders":true,"orderPriceMinTickSize":0.01,"outcomes":"[\"Up\",\"Down\"]","clobTokenIds":"[\"UPTOKEN\",\"DOWNTOKEN\"]"}`, slug)
	}))
	defer ts.Close()
	m, err := ResolveCurrentBTC5M(context.Background(), ResolveOptions{Now: now, BaseURL: ts.URL, HTTPClient: ts.Client()})
	if err != nil { t.Fatal(err) }
	if m.UpToken != "UPTOKEN" || m.DownToken != "DOWNTOKEN" || m.TickSize != "0.01" || m.ConditionID != "0xabc" {
		t.Fatalf("unexpected market %+v", m)
	}
}

func TestResolveRetriesTransientEOF(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 53, 28, 0, time.UTC)
	slug := CurrentBTC5MSlug(now)
	body := fmt.Sprintf(`{"id":"123","conditionId":"0xabc","slug":%q,"startDate":"2026-08-23T15:50:00Z","eventStartTime":"2026-08-23T15:50:00Z","endDate":"2026-08-23T15:55:00Z","active":true,"closed":false,"acceptingOrders":true,"orderPriceMinTickSize":0.01,"outcomes":"[\"Up\",\"Down\"]","clobTokenIds":"[\"UPTOKEN\",\"DOWNTOKEN\"]"}`, slug)
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, io.EOF
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
	m, err := ResolveCurrentBTC5M(context.Background(), ResolveOptions{
		Now: now, BaseURL: "https://example.invalid", HTTPClient: client,
		Attempts: 3, RetryDelay: time.Millisecond,
	})
	if err != nil { t.Fatal(err) }
	if calls.Load() != 2 { t.Fatalf("expected 2 attempts, got %d", calls.Load()) }
	if m.UpToken != "UPTOKEN" || m.DownToken != "DOWNTOKEN" { t.Fatalf("unexpected market %+v", m) }
}

func TestResolveRejectsWrongOutcomeOrderByName(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 53, 28, 0, time.UTC)
	slug := CurrentBTC5MSlug(now)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"123","conditionId":"0xabc","slug":%q,"startDate":"2026-08-23T15:50:00Z","endDate":"2026-08-23T15:55:00Z","active":true,"closed":false,"orderPriceMinTickSize":"0.01","outcomes":"[\"Down\",\"Up\"]","clobTokenIds":"[\"DOWN\",\"UP\"]"}`, slug)
	}))
	defer ts.Close()
	m, err := ResolveCurrentBTC5M(context.Background(), ResolveOptions{Now: now, BaseURL: ts.URL, HTTPClient: ts.Client()})
	if err != nil { t.Fatal(err) }
	if m.UpToken != "UP" || m.DownToken != "DOWN" { t.Fatalf("bad outcome mapping %+v", m) }
}
