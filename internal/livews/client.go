package livews

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func acceptKey(key string) string {
	s := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(s[:])
}

type Client struct {
	conn net.Conn
	br   *bufio.Reader
	mu   sync.Mutex
}

type bufferedConn struct {
	net.Conn
	br *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if c.br != nil && c.br.Buffered() > 0 {
		return c.br.Read(p)
	}
	return c.Conn.Read(p)
}

// Dial opens a direct WebSocket connection.
func Dial(rawURL string, timeout time.Duration) (*Client, error) {
	return DialWithProxy(rawURL, "", timeout)
}

// DialWithProxy opens a WebSocket connection, optionally tunneling the TCP
// connection through an HTTP CONNECT proxy. proxyURL must be empty or use
// http://. The proxy is transport-only; target TLS is still terminated and
// verified end-to-end by this client.
func DialWithProxy(rawURL, proxyURL string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported websocket scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("websocket URL missing host")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	targetAddr := net.JoinHostPort(host, port)
	conn, err := dialTransport(targetAddr, proxyURL, timeout)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "wss" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		_ = tlsConn.SetDeadline(time.Now().Add(timeout))
		if err := tlsConn.Handshake(); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("TLS handshake: %w", err)
		}
		_ = tlsConn.SetDeadline(time.Time{})
		conn = tlsConn
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nUser-Agent: btc5m-execution-engine/2\r\n\r\n", path, u.Host, key); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReaderSize(conn, 1<<20)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		conn.Close()
		return nil, fmt.Errorf("websocket upgrade status %d", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		conn.Close()
		return nil, errors.New("missing websocket Upgrade response")
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		conn.Close()
		return nil, errors.New("bad Sec-WebSocket-Accept")
	}
	// Do not close resp.Body after a 101 response. In Go it may wrap the upgraded connection.
	return &Client{conn: conn, br: br}, nil
}

func dialTransport(targetAddr, proxyURL string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return d.Dial("tcp", targetAddr)
	}
	p, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", err)
	}
	if p.Scheme != "http" {
		return nil, fmt.Errorf("unsupported proxy scheme %q; V2 supports http CONNECT only", p.Scheme)
	}
	proxyHost := p.Hostname()
	if proxyHost == "" {
		return nil, errors.New("proxy URL missing host")
	}
	proxyPort := p.Port()
	if proxyPort == "" {
		proxyPort = "80"
	}
	conn, err := d.Dial("tcp", net.JoinHostPort(proxyHost, proxyPort))
	if err != nil {
		return nil, fmt.Errorf("connect proxy: %w", err)
	}
	closeOnErr := func(e error) (net.Conn, error) {
		_ = conn.Close()
		return nil, e
	}

	var auth string
	if p.User != nil {
		user := p.User.Username()
		pass, _ := p.User.Password()
		auth = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\nUser-Agent: btc5m-execution-engine/2\r\n%s\r\n", targetAddr, targetAddr, auth); err != nil {
		return closeOnErr(fmt.Errorf("write CONNECT: %w", err))
	}
	br := bufio.NewReaderSize(conn, 32*1024)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return closeOnErr(fmt.Errorf("read CONNECT response: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return closeOnErr(fmt.Errorf("HTTP proxy CONNECT status %d", resp.StatusCode))
	}
	// Preserve any bytes that the buffered reader may already have read from the tunnel.
	return &bufferedConn{Conn: conn, br: br}, nil
}

func (c *Client) Close() error { return c.conn.Close() }
func (c *Client) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }
func (c *Client) WriteText(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, 0x1, payload, true)
}
func (c *Client) writeControl(op byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, op, payload, true)
}

func (c *Client) ReadText(maxPayload int) ([]byte, error) {
	for {
		op, payload, err := readFrame(c.br, maxPayload)
		if err != nil {
			return nil, err
		}
		switch op {
		case 0x1:
			return payload, nil
		case 0x8:
			_ = c.writeControl(0x8, nil)
			return nil, io.EOF
		case 0x9:
			if err := c.writeControl(0xA, payload); err != nil {
				return nil, err
			}
		case 0xA:
			continue
		default:
			return nil, fmt.Errorf("unsupported websocket opcode %d", op)
		}
	}
}

func writeFrame(w io.Writer, opcode byte, payload []byte, masked bool) error {
	if len(payload) > 16*1024*1024 {
		return fmt.Errorf("frame too large: %d", len(payload))
	}
	var hdr bytes.Buffer
	hdr.WriteByte(0x80 | (opcode & 0x0f))
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}
	n := len(payload)
	switch {
	case n <= 125:
		hdr.WriteByte(maskBit | byte(n))
	case n <= 65535:
		hdr.WriteByte(maskBit | 126)
		_ = binary.Write(&hdr, binary.BigEndian, uint16(n))
	default:
		hdr.WriteByte(maskBit | 127)
		_ = binary.Write(&hdr, binary.BigEndian, uint64(n))
	}
	out := payload
	if masked {
		key := [4]byte{}
		if _, err := rand.Read(key[:]); err != nil {
			return err
		}
		hdr.Write(key[:])
		out = append([]byte(nil), payload...)
		for i := range out {
			out[i] ^= key[i%4]
		}
	}
	if _, err := w.Write(hdr.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(out)
	return err
}

func readFrame(br *bufio.Reader, maxPayload int) (byte, []byte, error) {
	b0, err := br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b1, err := br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	if b0&0x80 == 0 {
		return 0, nil, errors.New("fragmented websocket frame unsupported")
	}
	op := b0 & 0x0f
	masked := b1&0x80 != 0
	n := uint64(b1 & 0x7f)
	if n == 126 {
		var v uint16
		if err := binary.Read(br, binary.BigEndian, &v); err != nil {
			return 0, nil, err
		}
		n = uint64(v)
	} else if n == 127 {
		if err := binary.Read(br, binary.BigEndian, &n); err != nil {
			return 0, nil, err
		}
	}
	if n > uint64(maxPayload) {
		return 0, nil, fmt.Errorf("frame payload %d exceeds max %d", n, maxPayload)
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(br, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, int(n))
	if _, err := io.ReadFull(br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return op, payload, nil
}
