package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 本文件是 RFC6455 的最小服务端 WebSocket 实现，
// 只使用 Go 标准库，避免项目引入外部依赖。

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	wsOpText  = 1
	wsOpClose = 8
	wsOpPing  = 9
	wsOpPong  = 10
)

var errWSClosed = errors.New("websocket closed")

type WSConn struct {
	conn   net.Conn
	br     *bufio.Reader
	wmu    sync.Mutex
	cmu    sync.Mutex
	closed bool
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// Upgrade 完成握手并把连接从 HTTP Server 劫持出来。
func Upgrade(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	if r.Method != http.MethodGet || !isWebSocketUpgrade(r) {
		return nil, errors.New("not a websocket handshake")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("response writer does not support hijack")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	return &WSConn{conn: conn, br: brw.Reader}, nil
}

// ReadMessage 读取一条完整消息（文本/二进制）。
// 内部处理 ping/pong/close 控制帧，并逐帧刷新读超时以维持心跳。
func (c *WSConn) ReadMessage() ([]byte, error) {
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

		var hdr [2]byte
		if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
			return nil, err
		}
		opcode := hdr[0] & 0x0F
		length := int(hdr[1] & 0x7F)
		masked := hdr[1]&0x80 != 0

		switch length {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return nil, err
			}
			length = int(binary.BigEndian.Uint16(ext[:]))
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return nil, err
			}
			v := binary.BigEndian.Uint64(ext[:])
			if v > 1<<20 {
				return nil, errors.New("frame too large")
			}
			length = int(v)
		}

		if !masked {
			return nil, errors.New("client frames must be masked")
		}
		var mask [4]byte
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return nil, err
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return nil, err
		}
		for i := range payload {
			payload[i] ^= mask[i&3]
		}

		switch opcode {
		case wsOpText, 2: // 文本 / 二进制都按消息处理
			return payload, nil
		case wsOpClose:
			_ = c.writeFrame(wsOpClose, []byte{})
			return nil, errWSClosed
		case wsOpPing:
			if err := c.writeFrame(wsOpPong, payload); err != nil {
				return nil, err
			}
		case wsOpPong:
			// 忽略
		default:
			return nil, errors.New("unsupported opcode")
		}
	}
}

func (c *WSConn) WriteText(p []byte) error {
	return c.writeFrame(wsOpText, p)
}

func (c *WSConn) WritePing() error {
	return c.writeFrame(wsOpPing, nil)
}

func (c *WSConn) writeFrame(opcode byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrameLocked(opcode, payload)
}

func (c *WSConn) writeFrameLocked(opcode byte, payload []byte) error {
	if c.closed {
		return errWSClosed
	}
	var hdr [10]byte
	hdr[0] = 0x80 | opcode
	n := len(payload)
	var hdrLen int
	switch {
	case n < 126:
		hdr[1] = byte(n)
		hdrLen = 2
	case n < 65536:
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:4], uint16(n))
		hdrLen = 4
	default:
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:10], uint64(n))
		hdrLen = 10
	}
	buf := make([]byte, 0, hdrLen+n)
	buf = append(buf, hdr[:hdrLen]...)
	buf = append(buf, payload...)
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(buf)
	return err
}

// Close 发送关闭帧并断开连接（幂等）。
func (c *WSConn) Close() {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	c.wmu.Lock()
	if !c.closed {
		_ = c.writeFrameLocked(wsOpClose, []byte{})
		c.closed = true
	}
	c.wmu.Unlock()
	_ = c.conn.Close()
}
