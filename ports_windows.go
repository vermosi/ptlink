//go:build windows

package main

import (
	"fmt"
	"strings"
)

func platformGamePorts() (map[int]string, map[int]string, error) {
	tcp, udp := map[int]string{}, map[int]string{}
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
		if !pids[f[len(f)-1]] {
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
