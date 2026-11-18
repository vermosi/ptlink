//go:build !darwin

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cfgPaths(gameDir string) []string {
	paths := []string{filepath.Join(gameDir, "pt.cfg")}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		// Windows may redirect writes under Program Files to VirtualStore.
		rel := strings.TrimPrefix(gameDir, filepath.VolumeName(gameDir))
		paths = append(paths, filepath.Join(la, "VirtualStore", rel, "pt.cfg"))
	}
	return paths
}

func launchGame(gameDir string) {
	exe := filepath.Join(gameDir, "pockettanks.exe")
	if _, err := os.Stat(exe); err != nil {
		log.Printf("Pocket Tanks not found next to PTLink. Start the game yourself.")
		return
	}
	c := exec.Command(exe)
	c.Dir = gameDir
	if err := c.Start(); err != nil {
		log.Printf("could not start game: %v", err)
	}
}

// findGameDir prefers the folder PTLink is in, then the default install folder.
func findGameDir() string {
	for _, d := range []string{exeDir(), `C:\Program Files (x86)\Pocket Tanks`, `C:\Program Files\Pocket Tanks`} {
		if _, err := os.Stat(filepath.Join(d, "pockettanks.exe")); err == nil {
			return d
		}
	}
	return exeDir()
}
