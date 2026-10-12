// Package ws implements the minimal subset of the WebSocket protocol
// (RFC 6455) needed by the snake server: the opening handshake, text
// frames, fragmentation, ping/pong and close handling. It depends only
// on the standard library so the project needs no third-party modules.
package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// writeTimeout bounds a single frame write so a stalled peer can
	// never hold the write lock forever.
	writeTimeout = 10 * time.Second

	// maxFrameSize caps incoming frames to keep memory bounded.
	maxFrameSize = 1 << 20 // 1 MiB
)

// Frame opcodes used by the server.
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// ErrNotWebsocket is returned when the request is not a valid
// WebSocket opening handshake.
var ErrNotWebsocket = errors.New("ws: not a websocket handshake")

// Conn is a server-side WebSocket connection.
type Conn struct {
	conn   net.Conn
	br     *bufio.Reader
	wmu    sync.Mutex
	closed atomic.Bool
}

// Upgrade performs the RFC 6455 opening handshake and hijacks the
// connection from the HTTP server.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, ErrNotWebsocket
	}
	if !tokenListContains(r.Header, "Connection", "upgrade") {
		return nil, ErrNotWebsocket
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, ErrNotWebsocket
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("ws: http server does not support hijacking")
	}
	netConn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	// The HTTP server may have set deadlines; clear them for the
	// long-lived socket connection.
	_ = netConn.SetDeadline(time.Time{})

	sum := sha1.Sum([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := netConn.Write([]byte(handshake)); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	br := bufio.NewReader(netConn)
	if brw != nil && brw.Reader != nil {
		br = brw.Reader // reuse bytes already buffered by net/http
	}
	return &Conn{conn: netConn, br: br}, nil
}

// tokenListContains reports whether the comma separated header values
// contain the given token (case-insensitive).
func tokenListContains(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// SetReadDeadline refreshes the idle deadline while waiting for frames.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

// ReadMessage blocks until one complete data message arrives. Control
// frames (ping/pong/close) are answered transparently.
func (c *Conn) ReadMessage() ([]byte, error) {
	var frag []byte
	for {
		fin, op, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case opText, opBinary:
			if fin {
				return payload, nil
			}
			frag = append(frag[:0], payload...)
		case opContinuation:
			if frag == nil {
				return nil, errors.New("ws: unexpected continuation frame")
			}
			frag = append(frag, payload...)
			if fin {
				msg := frag
				frag = nil
				return msg, nil
			}
		case opPing:
			if err := c.writeControl(opPong, payload); err != nil {
				return nil, err
			}
		case opPong:
			// ignore
		case opClose:
			_ = c.writeControl(opClose, nil)
			c.Close()
			return nil, io.EOF
		default:
			return nil, fmt.Errorf("ws: unsupported opcode %d", op)
		}
	}
}

// WriteText sends one unmasked text frame. Safe for concurrent use.
func (c *Conn) WriteText(b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrame(0x80|opText, b)
}

func (c *Conn) writeControl(op byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrame(0x80|op, payload)
}

// writeFrame writes a complete frame. The caller must hold wmu.
func (c *Conn) writeFrame(first byte, payload []byte) error {
	if c.closed.Load() {
		return errors.New("ws: connection closed")
	}
	// Bound the write so a stalled peer cannot wedge the connection;
	// Close() also unblocks an in-flight write by closing the socket.
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))

	n := len(payload)
	var hdr [10]byte
	hdr[0] = first
	h := 2
	switch {
	case n < 126:
		hdr[1] = byte(n)
	case n < 1<<16:
		hdr[1] = 126
		hdr[2] = byte(n >> 8)
		hdr[3] = byte(n)
		h = 4
	default:
		hdr[1] = 127
		hdr[2] = byte(n >> 56)
		hdr[3] = byte(n >> 48)
		hdr[4] = byte(n >> 40)
		hdr[5] = byte(n >> 32)
		hdr[6] = byte(n >> 24)
		hdr[7] = byte(n >> 16)
		hdr[8] = byte(n >> 8)
		hdr[9] = byte(n)
		h = 10
	}
	if _, err := c.conn.Write(hdr[:h]); err != nil {
		return err
	}
	if n > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Close tears down the underlying TCP connection. It deliberately does
// not take the write lock: closing the socket immediately unblocks any
// in-flight read or write instead of waiting on it.
func (c *Conn) Close() {
	if c.closed.Swap(true) {
		return
	}
	_ = c.conn.Close()
}

// readFrame reads a single frame and unmasks client payloads.
func (c *Conn) readFrame() (fin bool, op byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(c.br, hdr[:]); err != nil {
		return
	}
	if hdr[0]&0x70 != 0 {
		return false, 0, nil, errors.New("ws: reserved bits set")
	}
	fin = hdr[0]&0x80 != 0
	op = hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		length = uint64(ext[0])<<8 | uint64(ext[1])
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		for _, b := range ext {
			length = length<<8 | uint64(b)
		}
	}
	if length > maxFrameSize {
		return false, 0, nil, errors.New("ws: frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i&3]
		}
	}
	return
}
