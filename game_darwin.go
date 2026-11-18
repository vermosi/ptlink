//go:build darwin

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// gameApp finds Pocket Tanks.app.
func gameApp() string {
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(exeDir(), "Pocket Tanks.app"),
		"/Applications/Pocket Tanks.app",
		filepath.Join(home, "Applications", "Pocket Tanks.app"),
		filepath.Join(home, "Downloads", "Pocket Tanks.app"),
		filepath.Join(home, "Desktop", "Pocket Tanks.app"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

func findGameDir() string {
	if a := gameApp(); a != "" {
		return filepath.Dir(a)
	}
	return exeDir()
}

// cfgPaths lists where the Mac game may keep pt.cfg. We only update copies
// that already exist (setCfg keeps going if one is missing), plus the folder
// next to the app, which is where SDL ports usually look.
func cfgPaths(gameDir string) []string {
	home, _ := os.UserHomeDir()
	paths := []string{filepath.Join(gameDir, "pt.cfg")}
	if a := gameApp(); a != "" {
		paths = append(paths, filepath.Join(a, "Contents", "Resources", "pt.cfg"))
	}
	paths = append(paths,
		filepath.Join(home, "Library", "Application Support", "Pocket Tanks", "pt.cfg"),
		filepath.Join(home, "Library", "Preferences", "Pocket Tanks", "pt.cfg"))
	return paths
}

func launchGame(gameDir string) {
	app := gameApp()
	if app == "" {
		log.Printf("Pocket Tanks.app not found in Applications. Start the game yourself.")
		return
	}
	if err := exec.Command("open", app).Run(); err != nil {
		log.Printf("could not start game: %v", err)
	}
}
