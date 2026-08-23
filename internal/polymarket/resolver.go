package polymarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultGammaBaseURL = "https://gamma-api.polymarket.com"

type ResolvedMarket struct {
	ID              string    `json:"id"`
	ConditionID     string    `json:"condition_id"`
	Slug            string    `json:"slug"`
	UpToken         string    `json:"up_token"`
	DownToken       string    `json:"down_token"`
	TickSize        string    `json:"tick_size"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Active          bool      `json:"active"`
	Closed          bool      `json:"closed"`
	AcceptingOrders bool      `json:"accepting_orders"`
}

type ResolveOptions struct {
	Now       time.Time
	ProxyURL  string
	BaseURL   string
	Timeout   time.Duration
	HTTPClient *http.Client
}

type gammaMarket struct {
	ID                    string          `json:"id"`
	ConditionID           string          `json:"conditionId"`
	Slug                  string          `json:"slug"`
	StartDate             string          `json:"startDate"`
	EndDate               string          `json:"endDate"`
	EventStartTime        string          `json:"eventStartTime"`
	Active                bool            `json:"active"`
	Closed                bool            `json:"closed"`
	AcceptingOrders       bool            `json:"acceptingOrders"`
	OrderPriceMinTickSize json.RawMessage `json:"orderPriceMinTickSize"`
	Outcomes              string          `json:"outcomes"`
	ClobTokenIDs          string          `json:"clobTokenIds"`
}

// CurrentBTC5MSlug returns the deterministic slug for the 5-minute UTC slot
// containing now. The timestamp is the slot start in Unix seconds.
func CurrentBTC5MSlug(now time.Time) string {
	now = now.UTC()
	start := now.Unix() / 300 * 300
	return fmt.Sprintf("btc-updown-5m-%d", start)
}

func ResolveCurrentBTC5M(ctx context.Context, opts ResolveOptions) (ResolvedMarket, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultGammaBaseURL
	}
	client := opts.HTTPClient
	if client == nil {
		var err error
		client, err = resolverHTTPClient(opts.ProxyURL, opts.Timeout)
		if err != nil {
			return ResolvedMarket{}, err
		}
	}
	slug := CurrentBTC5MSlug(now)
	return resolveBySlug(ctx, client, base, slug, now)
}

func resolverHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxyURL) != "" {
		p, err := url.Parse(strings.TrimSpace(proxyURL))
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		if p.Scheme != "http" && p.Scheme != "https" {
			return nil, fmt.Errorf("unsupported HTTP resolver proxy scheme %q", p.Scheme)
		}
		tr.Proxy = http.ProxyURL(p)
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}

func resolveBySlug(ctx context.Context, client *http.Client, base, slug string, now time.Time) (ResolvedMarket, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/markets/slug/"+url.PathEscape(slug), nil)
	if err != nil {
		return ResolvedMarket{}, err
	}
	req.Header.Set("User-Agent", "btc5m-execution-engine/2")
	resp, err := client.Do(req)
	if err != nil {
		return ResolvedMarket{}, fmt.Errorf("gamma market request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ResolvedMarket{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ResolvedMarket{}, fmt.Errorf("gamma market status %d for slug %s: %s", resp.StatusCode, slug, strings.TrimSpace(string(body)))
	}
	var g gammaMarket
	if err := json.Unmarshal(body, &g); err != nil {
		return ResolvedMarket{}, fmt.Errorf("gamma market decode: %w", err)
	}
	if g.Slug != slug {
		return ResolvedMarket{}, fmt.Errorf("gamma slug mismatch: got %q want %q", g.Slug, slug)
	}
	if g.ID == "" || g.ConditionID == "" {
		return ResolvedMarket{}, errors.New("gamma market missing id or conditionId")
	}
	var outcomes []string
	if err := json.Unmarshal([]byte(g.Outcomes), &outcomes); err != nil {
		return ResolvedMarket{}, fmt.Errorf("decode outcomes: %w", err)
	}
	var tokens []string
	if err := json.Unmarshal([]byte(g.ClobTokenIDs), &tokens); err != nil {
		return ResolvedMarket{}, fmt.Errorf("decode clobTokenIds: %w", err)
	}
	if len(outcomes) != 2 || len(tokens) != 2 {
		return ResolvedMarket{}, fmt.Errorf("expected 2 outcomes/tokens, got %d/%d", len(outcomes), len(tokens))
	}
	upIdx, downIdx := -1, -1
	for i, o := range outcomes {
		switch strings.ToLower(strings.TrimSpace(o)) {
		case "up":
			upIdx = i
		case "down":
			downIdx = i
		}
	}
	if upIdx < 0 || downIdx < 0 || upIdx == downIdx {
		return ResolvedMarket{}, fmt.Errorf("unexpected BTC5m outcomes %v", outcomes)
	}
	if strings.TrimSpace(tokens[upIdx]) == "" || strings.TrimSpace(tokens[downIdx]) == "" {
		return ResolvedMarket{}, errors.New("empty CLOB token id")
	}
	start, err := parseMarketTime(firstNonEmpty(g.EventStartTime, g.StartDate))
	if err != nil {
		return ResolvedMarket{}, fmt.Errorf("parse start time: %w", err)
	}
	end, err := parseMarketTime(g.EndDate)
	if err != nil {
		return ResolvedMarket{}, fmt.Errorf("parse end time: %w", err)
	}
	if !now.Before(end) || now.Before(start) {
		return ResolvedMarket{}, fmt.Errorf("resolved market is not current: now=%s start=%s end=%s", now.Format(time.RFC3339Nano), start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	}
	if end.Sub(start) < 299*time.Second || end.Sub(start) > 301*time.Second {
		return ResolvedMarket{}, fmt.Errorf("unexpected BTC5m duration %s", end.Sub(start))
	}
	tick, err := parseTickSize(g.OrderPriceMinTickSize)
	if err != nil {
		return ResolvedMarket{}, err
	}
	if g.Closed || !g.Active {
		return ResolvedMarket{}, fmt.Errorf("current market inactive/closed: active=%v closed=%v", g.Active, g.Closed)
	}
	return ResolvedMarket{
		ID: g.ID, ConditionID: g.ConditionID, Slug: g.Slug,
		UpToken: tokens[upIdx], DownToken: tokens[downIdx], TickSize: tick,
		StartTime: start, EndTime: end, Active: g.Active, Closed: g.Closed,
		AcceptingOrders: g.AcceptingOrders,
	}, nil
}

func parseMarketTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	return time.Parse(time.RFC3339, s)
}

func parseTickSize(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", errors.New("missing orderPriceMinTickSize")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" { return "", errors.New("empty tick size") }
		if _, err := strconv.ParseFloat(s, 64); err != nil { return "", fmt.Errorf("invalid tick size %q", s) }
		return s, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return "", fmt.Errorf("decode tick size: %w", err)
	}
	s = n.String()
	if _, err := strconv.ParseFloat(s, 64); err != nil { return "", fmt.Errorf("invalid tick size %q", s) }
	return s, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" { return s }
	}
	return ""
}
