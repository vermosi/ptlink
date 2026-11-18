package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// discoverGamePorts lists the TCP (listening) and UDP ports opened by Pocket Tanks,
// mapped to the IP address each one is bound to.
// PTLINK_PORTS="tcp:1234,udp:5678" overrides discovery (used for testing).
func discoverGamePorts() (map[int]string, map[int]string, error) {
	tcp, udp := map[int]string{}, map[int]string{}
	if ov := os.Getenv("PTLINK_PORTS"); ov != "" {
		for _, f := range strings.Split(ov, ",") {
			kv := strings.SplitN(f, ":", 2)
			if len(kv) == 2 {
				p, _ := strconv.Atoi(kv[1])
				if kv[0] == "tcp" {
					tcp[p] = ""
				} else {
					udp[p] = ""
				}
			}
		}
		return tcp, udp, nil
	}
	return platformGamePorts()
}

func splitHostPort(s string) (string, int) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", 0
	}
	p, _ := strconv.Atoi(s[i+1:])
	return s[:i], p
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	hideWindow(c)
	return c
}
