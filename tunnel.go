package main

// Tunnels the game's local TCP/UDP traffic through the relay.
//
// Host side: the real game listens on its normal port(s). We discover them and
//            replay the joiner's traffic to 127.0.0.1:<port>.
// Join side: we pretend to be the host on JoinIP:<port> (default 127.0.0.2), and
//            the joiner's game connects there via its IP-to-IP option.

import (
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

type item struct {
	T   string `json:"t"`
	P   int    `json:"p,omitempty"`
	S   int    `json:"s,omitempty"`
	ID  int    `json:"id,omitempty"`
	D   string `json:"d,omitempty"`
	TCP []int  `json:"tcp,omitempty"`
	UDP []int  `json:"udp,omitempty"`
}

type envelope struct {
	F   string `json:"f"`           // "h" or "j"
	Sid string `json:"sid"`         // joiner session id
	K   string `json:"k,omitempty"` // handshake: sender's X25519 public key
	X   string `json:"x,omitempty"` // encrypted []item
}

type Tunnel struct {
	role   string // "h" or "j"
	sid    string // joiner session id this tunnel is bound to
	relay  *Relay
	mu     sync.Mutex
	queue  []item
	peerOK chan struct{}
	peerOn sync.Once
	onPeer func()
	code   string
	key    *sessionKey
	aead   cipher.AEAD
	peerK  string // peer public key we derived from
	qBytes int

	// host side
	hostTCP map[int]string // port -> ip the game listens on
	hostUDP map[int]string
	hUDP    map[[2]int]*net.UDPConn // (port, joiner src port) -> socket
	conns   map[int]net.Conn
	nextID  int

	// join side
	joinIP    string
	waitLocal bool                 // wait for our own game to open its ports before binding ours
	jUDP      map[int]*net.UDPConn // port -> our fake-host socket
	jUDPsrc   map[[2]int]*net.UDPAddr
	localIP   map[int]string // our own game UDP port -> bound IP
	jTCP      map[int]net.Listener
}

func newTunnel(role string) *Tunnel {
	return &Tunnel{role: role, peerOK: make(chan struct{}),
		hostTCP: map[int]string{}, hostUDP: map[int]string{},
		hUDP: map[[2]int]*net.UDPConn{}, conns: map[int]net.Conn{},
		jUDP: map[int]*net.UDPConn{}, jUDPsrc: map[[2]int]*net.UDPAddr{}, localIP: map[int]string{}, jTCP: map[int]net.Listener{}}
}

func (t *Tunnel) push(it item) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.qBytes+len(it.D) > maxQueue {
		return // relay can't keep up; drop rather than grow without limit
	}
	t.qBytes += len(it.D)
	t.queue = append(t.queue, it)
}

// sendHandshake publishes our public key (unencrypted by design).
func (t *Tunnel) sendHandshake() {
	t.relay.Broadcast(envelope{F: t.role, Sid: t.sid, K: t.key.pub})
}

// flushLoop batches queued items so we stay far below the relay's message limits.
func (t *Tunnel) flushLoop(every time.Duration) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	for {
		select {
		case <-t.relay.Done():
			return
		case <-tk.C:
		}
		t.mu.Lock()
		if t.aead == nil {
			t.mu.Unlock()
			continue // nothing leaves until the encrypted session exists
		}
		q := t.queue
		t.queue = nil
		t.qBytes = 0
		a, sid := t.aead, t.sid
		t.mu.Unlock()
		// keep each broadcast well under the 250 KB message cap
		for len(q) > 0 {
			n, size := 0, 0
			for n < len(q) && (n == 0 || size+len(q[n].D) < 150*1024) {
				size += len(q[n].D)
				n++
			}
			x, err := seal(a, t.role+"|"+sid, q[:n])
			if err == nil {
				err = t.relay.Broadcast(envelope{F: t.role, Sid: sid, X: x})
			}
			if err != nil {
				log.Printf("send error: %v", err)
			}
			q = q[n:]
		}
	}
}

func (t *Tunnel) onRelay(raw json.RawMessage) {
	if len(raw) > maxEnvelope {
		return
	}
	var e envelope
	if json.Unmarshal(raw, &e) != nil || e.F == t.role || (e.F != "h" && e.F != "j") || e.Sid == "" {
		return
	}
	t.mu.Lock()
	if t.role == "h" && t.sid == "" && e.K != "" {
		t.sid = e.Sid // first joiner to say hello gets the game
	}
	if e.Sid != t.sid {
		t.mu.Unlock()
		return // someone else with the same code; ignore
	}
	// Handshake: derive the session key from the first public key we see.
	if e.K != "" {
		if t.aead == nil {
			a, err := t.key.derive(e.K, t.code)
			if err != nil {
				t.mu.Unlock()
				return
			}
			t.aead, t.peerK = a, e.K
		}
		same := e.K == t.peerK
		t.mu.Unlock()
		if !same {
			log.Printf("Ignored a second player trying to take over this session.")
			return
		}
		if t.role == "h" {
			t.sendHandshake() // answer (again) so a joiner that missed it can finish
			t.markPeer()
			t.push(t.hostPortsItem())
		} else {
			t.markPeer()
		}
		return
	}
	a := t.aead
	t.mu.Unlock()
	if a == nil || e.X == "" {
		return
	}
	items, err := open(a, e.F+"|"+e.Sid, e.X)
	if err != nil {
		return // not from our peer, or tampered with
	}
	for _, it := range items {
		if len(it.D) > maxItemData*4/3+4 {
			continue
		}
		if t.role == "h" {
			t.hostHandle(it)
		} else {
			t.joinHandle(it)
		}
	}
}

func (t *Tunnel) markPeer() {
	t.peerOn.Do(func() {
		close(t.peerOK)
		if t.onPeer != nil {
			t.onPeer()
		}
	})
}

// ---------------- host side ----------------

func (t *Tunnel) hostPortsItem() item {
	t.mu.Lock()
	defer t.mu.Unlock()
	it := item{T: "ports"}
	for p := range t.hostTCP {
		it.TCP = append(it.TCP, p)
	}
	for p := range t.hostUDP {
		it.UDP = append(it.UDP, p)
	}
	sort.Ints(it.TCP)
	sort.Ints(it.UDP)
	return it
}

// watchPorts keeps the list of ports the game is using up to date.
func (t *Tunnel) watchPorts() {
	last := ""
	for {
		tcp, udp, err := discoverGamePorts()
		if err != nil {
			log.Printf("port scan: %v", err)
		} else {
			t.mu.Lock()
			t.hostTCP, t.hostUDP = tcp, udp
			t.mu.Unlock()
			it := t.hostPortsItem()
			s := fmt.Sprint(it.TCP, it.UDP)
			if s != last {
				if len(it.TCP)+len(it.UDP) == 0 {
					log.Printf("Waiting for Pocket Tanks to open a network game...")
				} else {
					log.Printf("Game is using TCP %v UDP %v", it.TCP, it.UDP)
				}
				last = s
				t.push(it)
			}
		}
		select {
		case <-t.relay.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func localTarget(ip string) string {
	if ip == "" || ip == "0.0.0.0" || ip == "::" || ip == "[::]" {
		return "127.0.0.1"
	}
	return ip
}

func (t *Tunnel) hostHandle(it item) {
	switch it.T {
	case "u":
		data, _ := base64.StdEncoding.DecodeString(it.D)
		key := [2]int{it.P, it.S}
		t.mu.Lock()
		c := t.hUDP[key]
		gip, allowed := t.hostUDP[it.P]
		ip := localTarget(gip)
		flows := len(t.hUDP)
		t.mu.Unlock()
		// Only ever deliver to the ports Pocket Tanks itself opened. Never let
		// the other player reach other programs on this computer.
		if !allowed || !validPort(it.S) {
			return
		}
		if c == nil {
			if flows >= maxUDPFlows {
				return
			}
			dst, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", ip, it.P))
			var err error
			c, err = net.DialUDP("udp4", nil, dst)
			if err != nil {
				log.Printf("udp to game: %v", err)
				return
			}
			t.mu.Lock()
			t.hUDP[key] = c
			t.mu.Unlock()
			go func() {
				buf := make([]byte, 65536)
				for {
					n, err := c.Read(buf)
					if err != nil {
						// Windows reports ICMP port-unreachable as a read error; keep going.
						select {
						case <-t.relay.Done():
							return
						default:
						}
						time.Sleep(50 * time.Millisecond)
						continue
					}
					t.push(item{T: "u", P: key[0], S: key[1], D: base64.StdEncoding.EncodeToString(buf[:n])})
				}
			}()
		}
		c.Write(data)
	case "o":
		t.mu.Lock()
		gip, allowed := t.hostTCP[it.P]
		ip := localTarget(gip)
		n := len(t.conns)
		t.mu.Unlock()
		if !allowed || n >= maxTCPConns {
			t.push(item{T: "c", ID: it.ID})
			return
		}
		c, err := net.DialTimeout("tcp4", fmt.Sprintf("%s:%d", ip, it.P), 5*time.Second)
		if err != nil {
			log.Printf("tcp to game port %d: %v", it.P, err)
			t.push(item{T: "c", ID: it.ID})
			return
		}
		t.addConn(it.ID, c)
	case "d":
		t.connWrite(it)
	case "c":
		t.connClose(it.ID, false)
	}
}

// ---------------- shared TCP plumbing ----------------

func (t *Tunnel) addConn(id int, c net.Conn) {
	t.mu.Lock()
	t.conns[id] = c
	t.mu.Unlock()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				t.push(item{T: "d", ID: id, D: base64.StdEncoding.EncodeToString(buf[:n])})
			}
			if err != nil {
				t.connClose(id, true)
				return
			}
		}
	}()
}

func (t *Tunnel) connWrite(it item) {
	t.mu.Lock()
	c := t.conns[it.ID]
	t.mu.Unlock()
	if c == nil {
		return
	}
	data, _ := base64.StdEncoding.DecodeString(it.D)
	c.Write(data)
}

func (t *Tunnel) connClose(id int, tell bool) {
	t.mu.Lock()
	c := t.conns[id]
	delete(t.conns, id)
	t.mu.Unlock()
	if c != nil {
		c.Close()
		if tell {
			t.push(item{T: "c", ID: id})
		}
	}
}

// ---------------- join side ----------------

func (t *Tunnel) joinHandle(it item) {
	switch it.T {
	case "a":
		t.joinAnnounce(it)
	case "ports":
		if t.waitLocal {
			go func() { t.waitForLocalGame(); t.joinHandle(item{T: "ports2", TCP: it.TCP, UDP: it.UDP}) }()
			return
		}
		fallthrough
	case "ports2":
		if len(it.TCP)+len(it.UDP) > maxPorts {
			return
		}
		for _, p := range it.TCP {
			if !validPort(p) {
				continue
			}
			t.joinListenTCP(p)
		}
		for _, p := range it.UDP {
			if validPort(p) {
				t.joinListenUDP(p)
			}
		}
		if len(it.TCP)+len(it.UDP) > 0 {
			log.Printf("Ready! In Pocket Tanks open Network Game: the host's game should appear in the LAN list (or use IP-to-IP and join %s)", t.joinIP)
		} else {
			log.Printf("Connected to host. Waiting for them to create a game in Pocket Tanks...")
		}
	case "u":
		t.mu.Lock()
		c := t.jUDP[it.P]
		src := t.jUDPsrc[[2]int{it.P, it.S}]
		t.mu.Unlock()
		if c == nil || src == nil {
			return
		}
		data, _ := base64.StdEncoding.DecodeString(it.D)
		c.WriteToUDP(data, src)
	case "d":
		t.connWrite(it)
	case "c":
		t.connClose(it.ID, false)
	}
}

func (t *Tunnel) joinListenTCP(p int) {
	t.mu.Lock()
	_, ok := t.jTCP[p]
	t.mu.Unlock()
	if ok {
		return
	}
	l, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", t.joinIP, p))
	if err != nil {
		log.Printf("could not listen on TCP %s:%d: %v", t.joinIP, p, err)
		return
	}
	t.mu.Lock()
	t.jTCP[p] = l
	t.mu.Unlock()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.mu.Lock()
			t.nextID++
			id := t.nextID
			t.mu.Unlock()
			t.push(item{T: "o", ID: id, P: p})
			t.addConn(id, c)
		}
	}()
}

func (t *Tunnel) joinListenUDP(p int) {
	t.mu.Lock()
	_, ok := t.jUDP[p]
	t.mu.Unlock()
	if ok {
		return
	}
	c, err := listenUDPMaybeShared(fmt.Sprintf("%s:%d", t.joinIP, p))
	if err != nil {
		log.Printf("could not listen on UDP %s:%d: %v", t.joinIP, p, err)
		return
	}
	t.mu.Lock()
	t.jUDP[p] = c
	t.mu.Unlock()
	go func() {
		buf := make([]byte, 65536)
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
			t.mu.Lock()
			t.jUDPsrc[[2]int{p, src.Port}] = src
			t.mu.Unlock()
			t.push(item{T: "u", P: p, S: src.Port, D: base64.StdEncoding.EncodeToString(buf[:n])})
		}
	}()
}

// waitForLocalGame waits (up to 2 minutes) until this PC's Pocket Tanks has
// opened its UDP ports, so our sockets never block the game from binding.
func (t *Tunnel) waitForLocalGame() {
	logged := false
	for i := 0; i < 60; i++ {
		if _, udp, err := discoverGamePorts(); err == nil && len(udp) > 0 {
			return
		}
		if !logged {
			log.Printf("Connected to host. Open Pocket Tanks and go to Network Game...")
			logged = true
		}
		time.Sleep(2 * time.Second)
	}
}
