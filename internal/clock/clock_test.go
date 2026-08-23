package clock

import "testing"

func TestMonotonicIncreases(t *testing.T) {
	a, err := MonotonicNS()
	if err != nil {
		t.Fatal(err)
	}
	b, err := MonotonicNS()
	if err != nil {
		t.Fatal(err)
	}
	if b < a {
		t.Fatalf("monotonic clock reversed: %d -> %d", a, b)
	}
}
