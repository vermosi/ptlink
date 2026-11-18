package main

import (
	"net"
	"testing"
	"time"
)

// A malicious joiner must not be able to reach programs other than the game.
func TestHostRefusesNonGamePorts(t *testing.T) {
	victim, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer victim.Close()
	hit := make(chan bool, 1)
	go func() {
		c, err := victim.Accept()
		if err == nil {
			c.Close()
			hit <- true
		}
	}()
	vp := victim.Addr().(*net.TCPAddr).Port

	uv, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer uv.Close()
	uhit := make(chan bool, 1)
	go func() {
		b := make([]byte, 10)
		uv.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := uv.ReadFrom(b); err == nil {
			uhit <- true
		}
	}()
	up := uv.LocalAddr().(*net.UDPAddr).Port

	tn := newTunnel("h")
	tn.hostTCP[28520] = "0.0.0.0" // the only game ports
	tn.hostUDP[28520] = "0.0.0.0"
	tn.hostHandle(item{T: "o", ID: 1, P: vp})
	tn.hostHandle(item{T: "u", P: up, S: 5000, D: "aGk="})
	select {
	case <-hit:
		t.Fatal("TCP reached a non-game port")
	case <-uhit:
		t.Fatal("UDP reached a non-game port")
	case <-time.After(1200 * time.Millisecond):
	}
}

func TestEncryptionRejectsOutsiders(t *testing.T) {
	h, j, eve := newSessionKey(), newSessionKey(), newSessionKey()
	ah, _ := h.derive(j.pub, "ABCD-EFGH")
	aj, _ := j.derive(h.pub, "ABCD-EFGH")
	ae, _ := eve.derive(h.pub, "ABCD-EFGH") // knows the code and the host's public key
	x, _ := seal(aj, "j|sid", []item{{T: "u", P: 1, D: "aGk="}})
	if it, err := open(ah, "j|sid", x); err != nil || len(it) != 1 {
		t.Fatal("host could not read joiner traffic", err)
	}
	if _, err := open(ae, "j|sid", x); err == nil {
		t.Fatal("eavesdropper decrypted traffic")
	}
	forged, _ := seal(ae, "j|sid", []item{{T: "o", P: 22}})
	if _, err := open(ah, "j|sid", forged); err == nil {
		t.Fatal("host accepted forged traffic")
	}
	if _, err := open(ah, "h|sid", x); err == nil {
		t.Fatal("message accepted with the wrong direction")
	}
}

func TestCodeAndLinkValidation(t *testing.T) {
	for _, c := range []string{"ABCD-EFGH", "2345-6789"} {
		if !validCode(c) {
			t.Fatal("rejected valid", c)
		}
	}
	for _, c := range []string{"", "ABCD", "ABCD-EFGH-X", "abcd-efgh", "ABCD-EFG1", "../X-YYYY", "ABCD-EF H"} {
		if validCode(c) {
			t.Fatal("accepted invalid", c)
		}
	}
	if _, _, code, _ := parseLink("ptlink://join/ABCD-EFGH%20-relay%20ws://evil"); validCode(code) {
		t.Fatal("crafted link produced a valid code")
	}
}

func TestQueueIsBounded(t *testing.T) {
	tn := newTunnel("h")
	big := string(make([]byte, 60000))
	for i := 0; i < 100; i++ {
		tn.push(item{T: "d", D: big})
	}
	if tn.qBytes > maxQueue {
		t.Fatal("queue grew past its limit")
	}
}
