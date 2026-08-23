package polymarket

import "testing"

func TestEmbeddedBoundarySemantics(t *testing.T) {
	cases := []struct {
		raw, side string
		wantNil   bool
		wantTicks int64
		wantErr   bool
	}{
		{"0", "bid", true, 0, false},
		{"1", "ask", true, 0, false},
		{"0.99", "bid", false, 99, false},
		{"0.01", "ask", false, 1, false},
		{"1", "bid", false, 0, true},
		{"0", "ask", false, 0, true},
	}
	for _, tc := range cases {
		got, err := NormalizeEmbeddedBest(tc.raw, tc.side, "0.01")
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s %s: wanted error", tc.side, tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s %s: %v", tc.side, tc.raw, err)
		}
		if tc.wantNil {
			if got != nil {
				t.Fatalf("%s %s: wanted nil", tc.side, tc.raw)
			}
			continue
		}
		if got == nil || *got != tc.wantTicks {
			t.Fatalf("%s %s: got %v", tc.side, tc.raw, got)
		}
	}
}
