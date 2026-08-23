package mockws

// Package mockws implements the tiny RFC6455 subset needed by the V1 loopback
// fault-injection harness. It is intentionally not used for production/live
// Internet connectivity.

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Upgrade accepts a websocket upgrade on an already-accepted TCP connection.
// Only loopback mock servers use this function.
func Upgrade(conn net.Conn) (*bufio.ReadWriter, error) {
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") || !headerHasToken(req.Header.Get("Connection"), "upgrade") {
		return nil, errors.New("not a websocket upgrade")
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	bw := bufio.NewWriter(conn)
	fmt.Fprintf(bw, "HTTP/1.1 101 Switching Protocols\r\n")
	fmt.Fprintf(bw, "Upgrade: websocket\r\n")
	fmt.Fprintf(bw, "Connection: Upgrade\r\n")
	fmt.Fprintf(bw, "Sec-WebSocket-Accept: %s\r\n\r\n", acceptKey(key))
	if err := bw.Flush(); err != nil {
		return nil, err
	}
	return bufio.NewReadWriter(br, bw), nil
}

func headerHasToken(v, token string) bool {
	for _, p := range strings.Split(v, ",") {
		if strings.EqualFold(strings.TrimSpace(p), token) {
			return true
		}
	}
	return false
}

// Dial opens a loopback-only websocket connection.
func Dial(rawURL string, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	if u.Scheme != "ws" {
		return nil, nil, fmt.Errorf("mockws only supports ws://, got %s", u.Scheme)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, nil, fmt.Errorf("V1 mockws refuses non-loopback host %q", host)
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return nil, nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n\r\n", path, u.Host, key)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, nil, fmt.Errorf("upgrade status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != acceptKey(key) {
		conn.Close()
		return nil, nil, fmt.Errorf("bad Sec-WebSocket-Accept")
	}
	return conn, br, nil
}

// WriteTextServer writes an unmasked server-to-client text frame.
func WriteTextServer(w io.Writer, payload []byte) error {
	return writeFrame(w, 0x1, payload, false)
}

// WriteCloseServer writes a close frame. The caller may also just close TCP to
// simulate an abrupt transport fault.
func WriteCloseServer(w io.Writer) error {
	return writeFrame(w, 0x8, nil, false)
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

// ReadTextClient reads one complete server-to-client text frame. Fragmentation
// is intentionally rejected in V1 so the failure is explicit.
func ReadTextClient(br *bufio.Reader, maxPayload int) ([]byte, error) {
	opcode, payload, err := readFrame(br, maxPayload)
	if err != nil {
		return nil, err
	}
	switch opcode {
	case 0x1:
		return payload, nil
	case 0x8:
		return nil, io.EOF
	default:
		return nil, fmt.Errorf("unexpected websocket opcode %d", opcode)
	}
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
		return 0, nil, errors.New("fragmented frame unsupported")
	}
	opcode := b0 & 0x0f
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
	return opcode, payload, nil
}
