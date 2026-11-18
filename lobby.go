package main

// Public game list: hosts announce themselves on the shared "ptlink-lobby"
// channel using Realtime presence. The website lists everything there.
// Presence disappears automatically when PTLink closes or loses connection.

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type Lobby struct {
	r     *Relay
	state map[string]any
}

func playerName(gameDir string) string {
	for _, p := range []string{filepath.Join(gameDir, "pt.cfg"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "VirtualStore", "Program Files (x86)", "Pocket Tanks", "pt.cfg")} {
		if b, err := os.ReadFile(p); err == nil {
			if m := regexp.MustCompile(`netPlayerName="([^"]+)"`).FindSubmatch(b); m != nil {
				return string(m[1])
			}
		}
	}
	return "Player"
}

func announceGame(code, name string) *Lobby {
	r, err := ConnectRelayPresence(RelayURL, RelayKey, "ptlink-lobby", code, func(json.RawMessage) {})
	if err != nil {
		log.Printf("Could not list your game publicly (%v). Friends can still join with the code.", err)
		return nil
	}
	l := &Lobby{r: r, state: map[string]any{
		"code": code, "host": name, "status": "waiting", "version": 1,
		"created_at": time.Now().UTC().Format(time.RFC3339)}}
	r.Track(l.state)
	log.Printf("Your game is listed publicly on the PTLink game browser.")
	return l
}

func (l *Lobby) SetStatus(s string) {
	if l == nil {
		return
	}
	l.state["status"] = s
	l.r.Track(l.state)
}
