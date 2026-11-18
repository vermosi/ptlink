//go:build darwin

package main

import (
	"strings"
)

// platformGamePorts uses lsof to list the sockets of the running Pocket Tanks app.
func platformGamePorts() (map[int]string, map[int]string, error) {
	tcp, udp := map[int]string{}, map[int]string{}
	out, _ := hiddenCmd("pgrep", "-x", "Pocket Tanks").Output()
	pids := strings.Fields(string(out))
	if len(pids) == 0 {
		return tcp, udp, nil
	}
	lo, err := hiddenCmd("lsof", "-nP", "-a", "-p", strings.Join(pids, ","), "-i").Output()
	if err != nil && len(lo) == 0 {
		return tcp, udp, nil // lsof exits 1 when the game has no sockets yet
	}
	for _, line := range strings.Split(string(lo), "\n") {
		f := strings.Fields(line)
		// COMMAND... PID USER FD TYPE DEVICE SIZE/OFF NODE NAME [(STATE)]
		node := -1
		for i, x := range f {
			if x == "UDP" || x == "TCP" {
				node = i
			}
		}
		if node < 0 || node+1 >= len(f) {
			continue
		}
		name := f[node+1]
		if strings.Contains(name, "->") || strings.HasPrefix(name, "[") {
			continue
		}
		ip, port := splitHostPort(name)
		if port == 0 {
			continue
		}
		if ip == "*" {
			ip = "0.0.0.0"
		}
		if f[node] == "UDP" {
			udp[port] = ip
		} else if strings.Contains(line, "(LISTEN)") {
			tcp[port] = ip
		}
	}
	return tcp, udp, nil
}
