package main

// LAN-mode support: Pocket Tanks finds LAN games with UDP broadcasts.
// We catch those broadcasts by binding the game's UDP ports on this PC's LAN
// addresses (more specific than the game's own 0.0.0.0 bind, so Windows still
// delivers normal traffic to the game) and pass them through the relay.
//
// Join side: a broadcast from the joiner's game becomes a normal "u" packet to
//            the host's game; the reply comes back from 127.0.0.2, so the
//            joiner's game lists the host's game at 127.0.0.2.
// Host side: broadcast announcements from the host's game are sent as "a" and
//            delivered to the joiner's game from 127.0.0.2.

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strings"
	"syscall"
	"time"
)

func lanIPv4s() []net.IP {
	var out []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip4 := n.IP.To4(); ip4 != nil && !ip4.IsLoopback() && !ip4.IsLinkLocalUnicast() {
				out = append(out, ip4)
			}
		}
	}
	return out
}

// lanCatcher waits until this PC's own Pocket Tanks has opened its UDP ports,
// then listens for its broadcasts on the LAN addresses.
func (t *Tunnel) lanCatcher() {
	bound := map[string]bool{}
	for {
		select {
		case <-t.relay.Done():
			return
		case <-time.After(2 * time.Second):
		}
		_, udp, err := discoverGamePorts()
		if err != nil {
			continue
		}
		for p, gameIP := range udp {
			key := fmt.Sprint(p)
			if bound[key] {
				continue
			}
			bound[key] = true
			t.mu.Lock()
			t.localIP[p] = gameIP
			t.mu.Unlock()
			log.Printf("LAN: your game uses UDP %s:%d", gameIP, p)
			if c := listenBroadcast(p, gameIP); c != nil {
				go t.lanRead(c, p)
			}
		}
	}
}

// listenBroadcast tries, in order of safety, to get a socket that receives the
// game's broadcasts on port p without stealing the game's own traffic.
func listenBroadcast(p int, gameIP string) *net.UDPConn {
	type try struct {
		ip    net.IP
		reuse bool
	}
	var tries []try
	for _, ip := range lanIPv4s() {
		if ip.String() != gameIP {
			tries = append(tries, try{ip, false})
		}
	}
	for _, ip := range lanIPv4s() {
		tries = append(tries, try{ip, true})
	}
	tries = append(tries, try{net.IPv4zero, true})
	var lastErr error
	for _, tr := range tries {
		lc := net.ListenConfig{}
		if tr.reuse {
			lc.Control = func(_, _ string, rc syscall.RawConn) error {
				var e error
				rc.Control(func(fd uintptr) { e = setReuse(fd) })
				return e
			}
		}
		pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("%s:%d", tr.ip, p))
		if err != nil {
			lastErr = err
			continue
		}
		log.Printf("LAN: watching for game broadcasts on %s:%d (shared=%v)", tr.ip, p, tr.reuse)
		return pc.(*net.UDPConn)
	}
	log.Printf("LAN: can't watch port %d: %v", p, lastErr)
	if lastErr != nil && strings.Contains(lastErr.Error(), "forbidden") {
		log.Printf("LAN: if PTLink is running as administrator, close it and run it normally (double-click).")
	}
	return nil
}

func (t *Tunnel) lanRead(c *net.UDPConn, port int) {
	buf := make([]byte, 65536)
	seen := 0
	for {
		n, src, err := c.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-t.relay.Done():
				return
			default:
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if src.IP.IsLoopback() || src.Port == 0 {
			continue // our own relayed traffic, not a broadcast
		}
		if seen < 3 {
			log.Printf("LAN: caught %d-byte broadcast from the game (port %d -> %d)", n, src.Port, port)
			seen++
		}
		d := base64.StdEncoding.EncodeToString(buf[:n])
		if t.role == "j" {
			// Replies must reach the game over loopback.
			t.ensureJoinUDP(port)
			t.mu.Lock()
			t.jUDPsrc[[2]int{port, src.Port}] = t.gameAddr(src.Port)
			t.mu.Unlock()
			t.push(item{T: "u", P: port, S: src.Port, D: d})
		} else {
			t.push(item{T: "a", P: port, S: src.Port, D: d})
		}
	}
}

// ensureJoinUDP makes sure we have the 127.0.0.2:<port> "fake host" socket.
func (t *Tunnel) ensureJoinUDP(port int) *net.UDPConn {
	t.joinListenUDP(port)
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.jUDP[port]
}

// joinAnnounce delivers a host broadcast to the joiner's game from 127.0.0.2.
func (t *Tunnel) joinAnnounce(it item) {
	c := t.ensureJoinUDP(it.P)
	if c == nil {
		return
	}
	data, _ := base64.StdEncoding.DecodeString(it.D)
	dst := t.gameAddr(it.P)
	t.mu.Lock()
	if t.jUDPsrc[[2]int{it.P, it.S}] == nil {
		t.jUDPsrc[[2]int{it.P, it.S}] = dst
	}
	t.mu.Unlock()
	c.WriteToUDP(data, dst)
}

// gameAddr is where this PC's own game can be reached on UDP port p.
func (t *Tunnel) gameAddr(p int) *net.UDPAddr {
	t.mu.Lock()
	ip := t.localIP[p]
	t.mu.Unlock()
	if ip == "" {
		// The game's source port may not be one of its listening ports; use
		// whichever address its main socket is bound to.
		t.mu.Lock()
		for _, v := range t.localIP {
			ip = v
			break
		}
		t.mu.Unlock()
	}
	return &net.UDPAddr{IP: net.ParseIP(localTarget(ip)).To4(), Port: p}
}
