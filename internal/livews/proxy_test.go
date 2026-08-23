package livews_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"btc5m-execution-engine/internal/livews"
	"btc5m-execution-engine/internal/mockws"
)

func TestDialWithHTTPConnectProxy(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		conn, err := target.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rw, err := mockws.Upgrade(conn)
		if err != nil {
			return
		}
		_ = mockws.WriteTextServer(rw, []byte("proxy-ok"))
		_ = rw.Flush()
	}()

	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	go func() {
		conn, err := proxy.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		req, err := http.ReadRequest(br)
		if err != nil || req.Method != http.MethodConnect {
			return
		}
		upstream, err := net.DialTimeout("tcp", req.Host, 2*time.Second)
		if err != nil {
			_, _ = fmt.Fprint(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
			return
		}
		defer upstream.Close()
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, br)
			close(done)
		}()
		_, _ = io.Copy(conn, upstream)
		<-done
	}()

	client, err := livews.DialWithProxy(
		"ws://"+target.Addr().String()+"/ws",
		"http://"+proxy.Addr().String(),
		3*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	msg, err := client.ReadText(1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != "proxy-ok" {
		t.Fatalf("unexpected payload %q", msg)
	}
}

func TestDialWithProxyRejectsUnsupportedScheme(t *testing.T) {
	_, err := livews.DialWithProxy("ws://127.0.0.1:1/ws", "socks5://127.0.0.1:7897", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected unsupported proxy scheme error")
	}
}
