package main

// Minimal WebSocket client (RFC 6455), standard library only.

import (
	"bufio"
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

type wsConn struct {
	c  net.Conn
	br *bufio.Reader
	wm sync.Mutex
}

func wsDial(raw string) (*wsConn, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host := u.Host
	var c net.Conn
	d := &net.Dialer{Timeout: 15 * time.Second}
	switch u.Scheme {
	case "wss":
		if !strings.Contains(host, ":") {
			host += ":443"
		}
		c, err = tls.DialWithDialer(d, "tcp", host, &tls.Config{ServerName: u.Hostname()})
	case "ws":
		if !strings.Contains(host, ":") {
			host += ":80"
		}
		c, err = d.Dial("tcp", host)
	default:
		return nil, fmt.Errorf("bad scheme %q", u.Scheme)
	}
	if err != nil {
		return nil, err
	}
	kb := make([]byte, 16)
	rand.Read(kb)
	key := base64.StdEncoding.EncodeToString(kb)
	req := "GET " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host +
		"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key +
		"\r\nSec-WebSocket-Version: 13\r\nUser-Agent: ptlink\r\n\r\n"
	if _, err = io.WriteString(c, req); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReaderSize(c, 64*1024)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	if resp.StatusCode != 101 {
		c.Close()
		return nil, fmt.Errorf("websocket handshake failed: %s", resp.Status)
	}
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(h[:]) {
		c.Close()
		return nil, errors.New("websocket bad accept key")
	}
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) writeFrame(op byte, p []byte) error {
	w.wm.Lock()
	defer w.wm.Unlock()
	hdr := []byte{0x80 | op}
	n := len(p)
	switch {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n < 65536:
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 0x80|127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	var mask [4]byte
	rand.Read(mask[:])
	hdr = append(hdr, mask[:]...)
	buf := make([]byte, len(hdr)+n)
	copy(buf, hdr)
	for i := 0; i < n; i++ {
		buf[len(hdr)+i] = p[i] ^ mask[i&3]
	}
	_, err := w.c.Write(buf)
	return err
}

func (w *wsConn) WriteText(p []byte) error { return w.writeFrame(1, p) }

// ReadMessage returns the next complete text/binary message.
func (w *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return nil, err
		}
		fin := h[0]&0x80 != 0
		op := h[0] & 0x0f
		masked := h[1]&0x80 != 0
		n := uint64(h[1] & 0x7f)
		if n == 126 {
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		} else if n == 127 {
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return nil, err
			}
		}
		if n > 16<<20 {
			return nil, errors.New("websocket frame too large")
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.br, p); err != nil {
			return nil, err
		}
		if masked {
			for i := range p {
				p[i] ^= mask[i&3]
			}
		}
		switch op {
		case 8:
			return nil, errors.New("websocket closed by server")
		case 9:
			w.writeFrame(10, p)
			continue
		case 10:
			continue
		}
		msg = append(msg, p...)
		if fin {
			return msg, nil
		}
	}
}

func (w *wsConn) Close() error { return w.c.Close() }
