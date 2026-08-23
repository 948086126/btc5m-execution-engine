package mocksource

import "testing"

func TestRefusesNonLoopbackBind(t *testing.T) {
	s := New(Config{Addr: "0.0.0.0:0", Source: "M2"})
	if err := s.Listen(); err == nil {
		_ = s.Close()
		t.Fatal("expected non-loopback bind refusal")
	}
}
