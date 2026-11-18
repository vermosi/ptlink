package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// discoverGamePorts lists the TCP (listening) and UDP ports opened by pockettanks.exe.
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
	pids, err := gamePIDs()
	if err != nil || len(pids) == 0 {
		return tcp, udp, err
	}
	out, err := hiddenCmd("netstat", "-ano").Output()
	if err != nil {
		return tcp, udp, fmt.Errorf("netstat: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid := f[len(f)-1]
		if !pids[pid] {
			continue
		}
		ip, port := splitHostPort(f[1])
		if port == 0 || strings.HasPrefix(ip, "[") {
			continue
		}
		switch f[0] {
		case "TCP":
			if len(f) >= 5 && f[3] == "LISTENING" {
				tcp[port] = ip
			}
		case "UDP":
			udp[port] = ip
		}
	}
	return tcp, udp, nil
}

func splitHostPort(s string) (string, int) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", 0
	}
	p, _ := strconv.Atoi(s[i+1:])
	return s[:i], p
}

func gamePIDs() (map[string]bool, error) {
	out, err := hiddenCmd("tasklist", "/FI", "IMAGENAME eq pockettanks.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, fmt.Errorf("tasklist: %v", err)
	}
	pids := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\",\"")
		if len(f) >= 2 && strings.EqualFold(strings.Trim(f[0], "\" \r"), "pockettanks.exe") {
			pids[strings.Trim(f[1], "\" ")] = true
		}
	}
	return pids, nil
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	hideWindow(c)
	return c
}
