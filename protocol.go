package main

// ptlink:// links let the website start PTLink with one click:
//   ptlink://join/ABCD-EFGH   ptlink://host   ptlink://host-private
// Registered per-user (HKCU), so no administrator rights are needed.

import (
	"log"
	"os"
	"runtime"
	"strings"
)

func registerProtocol() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	key := `HKCU\Software\Classes\ptlink`
	cmds := [][]string{
		{"add", key, "/ve", "/d", "URL:PTLink", "/f"},
		{"add", key, "/v", "URL Protocol", "/d", "", "/f"},
		{"add", key + `\shell\open\command`, "/ve", "/d", `"` + exe + `" "%1"`, "/f"},
	}
	for _, c := range cmds {
		if err := hiddenCmd("reg", c...).Run(); err != nil {
			log.Printf("note: couldn't set up website buttons (%v)", err)
			return
		}
	}
}

// parseLink turns a ptlink:// argument into (host, private, joinCode).
func parseLink(arg string) (bool, bool, string, bool) {
	if !strings.HasPrefix(strings.ToLower(arg), "ptlink://") {
		return false, false, "", false
	}
	rest := strings.Trim(arg[len("ptlink://"):], "/ ")
	parts := strings.SplitN(rest, "/", 2)
	switch strings.ToLower(parts[0]) {
	case "host":
		return true, false, "", true
	case "host-private":
		return true, true, "", true
	case "join":
		if len(parts) == 2 {
			return false, false, strings.ToUpper(parts[1]), true
		}
	}
	return false, false, "", false
}
