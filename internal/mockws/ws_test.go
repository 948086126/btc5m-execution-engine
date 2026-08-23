package mockws

import (
	"net"
	"testing"
	"time"
)

func TestLoopbackRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		rw, err := Upgrade(c)
		if err != nil {
			return
		}
		_ = WriteTextServer(rw.Writer, []byte(`{"ok":true}`))
		_ = rw.Flush()
	}()
	c, br, err := Dial("ws://"+ln.Addr().String()+"/ws", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, err := ReadTextClient(br, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"ok":true}` {
		t.Fatalf("got %s", b)
	}
}

func TestRefusesNonLoopback(t *testing.T) {
	if _, _, err := Dial("ws://8.8.8.8:80/ws", 10*time.Millisecond); err == nil {
		t.Fatal("expected non-loopback refusal")
	}
}
