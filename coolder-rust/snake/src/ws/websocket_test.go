package ws

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// dialAndHandshake connects to url the way a browser would: raw TCP,
// manual opening handshake, verification of the accept key.
func dialAndHandshake(t *testing.T, url string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(url, "http://"), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("key: %v", err)
	}
	secKey := base64.StdEncoding.EncodeToString(key)
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + url + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: keep-alive, Upgrade\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("handshake status = %q", status)
	}
	headers := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.Index(line, ":"); i > 0 {
			headers[strings.ToLower(strings.TrimSpace(line[:i]))] = strings.TrimSpace(line[i+1:])
		}
	}
	sum := sha1.Sum([]byte(secKey + websocketGUID))
	want := base64.StdEncoding.EncodeToString(sum[:])
	if headers["sec-websocket-accept"] != want {
		t.Fatalf("accept = %q, want %q", headers["sec-websocket-accept"], want)
	}
	return conn, br
}

// clientFrame builds one masked client-to-server frame.
func clientFrame(op byte, payload []byte) []byte {
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	frame := []byte{0x80 | op}
	n := len(payload)
	switch {
	case n < 126:
		frame = append(frame, 0x80|byte(n))
	case n < 1<<16:
		frame = append(frame, 0x80|126, byte(n>>8), byte(n))
	default:
		frame = append(frame, 0x80|127,
			byte(n>>56), byte(n>>48), byte(n>>40), byte(n>>32),
			byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	frame = append(frame, mask...)
	for i, b := range payload {
		frame = append(frame, b^mask[i&3])
	}
	return frame
}

// readServerFrame reads one server-to-client frame (always unmasked).
func readServerFrame(t *testing.T, br *bufio.Reader) (op byte, payload []byte) {
	t.Helper()
	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		t.Fatalf("read frame header: %v", err)
	}
	op = hdr[0] & 0x0f
	if hdr[1]&0x80 != 0 {
		t.Fatal("server frames must not be masked")
	}
	n := int(hdr[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(br, ext[:]); err != nil {
			t.Fatalf("read extended length: %v", err)
		}
		n = int(ext[0])<<8 | int(ext[1])
	case 127:
		t.Fatal("unexpected 64-bit length in test")
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(br, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	return op, payload
}

func TestUpgradeAndEcho(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Upgrade(w, r)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		msg, err := c.ReadMessage()
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if string(msg) != "hello 蛇" {
			t.Errorf("server got %q", msg)
		}
		_ = c.WriteText([]byte("world"))
	}))
	defer srv.Close()

	conn, br := dialAndHandshake(t, srv.URL)
	if _, err := conn.Write(clientFrame(opText, []byte("hello 蛇"))); err != nil {
		t.Fatalf("send: %v", err)
	}
	op, payload := readServerFrame(t, br)
	if op != opText || string(payload) != "world" {
		t.Fatalf("echo = op %d %q, want text %q", op, payload, "world")
	}
}

func TestControlFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Upgrade(w, r)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			if _, err := c.ReadMessage(); err != nil {
				return // close frame or connection error ends the loop
			}
		}
	}))
	defer srv.Close()

	conn, br := dialAndHandshake(t, srv.URL)

	// ping -> pong with identical payload
	if _, err := conn.Write(clientFrame(opPing, []byte("hb"))); err != nil {
		t.Fatalf("send ping: %v", err)
	}
	op, payload := readServerFrame(t, br)
	if op != opPong || string(payload) != "hb" {
		t.Fatalf("pong = op %d %q", op, payload)
	}

	// close -> close echo, then the server drops the connection
	if _, err := conn.Write(clientFrame(opClose, []byte{0x03, 0xE8})); err != nil {
		t.Fatalf("send close: %v", err)
	}
	op, _ = readServerFrame(t, br)
	if op != opClose {
		t.Fatalf("close echo = op %d", op)
	}
}
