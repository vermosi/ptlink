// PTLink - play Pocket Tanks online with a short code.
// Open source (MIT). https://github.com/ (your repo here)
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Default relay (a free Lovable Cloud / Supabase project). These values are
// public by design: they only allow joining Realtime broadcast channels.
// Override with ptlink.json next to the exe: {"relay_url": "...", "relay_key": "..."}
var (
	RelayURL = "https://c--87c15b48-02fd-4f09-ac46-4144f0f85805-prod.lovable.cloud"
	RelayKey = "sb_publishable_MKrYWdA7g0N_mKkN0vGy6g_FCJHmDfm"
)

const codeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func randCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = codeChars[int(b[i])%len(codeChars)]
	}
	return string(b)
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func loadConfig() {
	b, err := os.ReadFile(filepath.Join(exeDir(), "ptlink.json"))
	if err != nil {
		return
	}
	var c struct {
		URL string `json:"relay_url"`
		Key string `json:"relay_key"`
	}
	if json.Unmarshal(b, &c) == nil {
		if c.URL != "" {
			RelayURL = c.URL
		}
		if c.Key != "" {
			RelayKey = c.Key
		}
	}
}

// setCfg turns on Pocket Tanks' built-in IP-to-IP mode in pt.cfg.
func setCfg(gameDir, ip string) {
	paths := cfgPaths(gameDir)
	wrote := false
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil && i > 0 && wrote {
			continue // no VirtualStore copy; nothing to update
		}
		s := string(b)
		if !strings.Contains(s, "<PT_CONFIG") {
			s = "<PT_CONFIG\r\n/>\r\n"
		}
		s = setAttr(s, "networkIPSupport", "1")
		// Let Mac and Windows copies of the game play each other.
		s = setAttr(s, "networkExeCRC", "0")
		if ip != "" {
			s = setAttr(s, "networkIP2IPAddress", ip)
		}
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(s), 0644); err == nil {
			wrote = true
		} else if i == 0 {
			log.Printf("note: could not write %s (%v), trying VirtualStore", p, err)
		}
	}
	if !wrote {
		log.Printf("WARNING: could not update pt.cfg. Run PTLink as administrator once.")
	}
}

func setAttr(s, k, v string) string {
	re := regexp.MustCompile(k + `="[^"]*"`)
	if re.MatchString(s) {
		return re.ReplaceAllString(s, k+`="`+v+`"`)
	}
	i := strings.LastIndex(s, "/>")
	if i < 0 {
		return s
	}
	return s[:i] + "    " + k + `="` + v + "\"\r\n" + s[i:]
}

func main() {
	host := flag.Bool("host", false, "host a game")
	join := flag.String("join", "", "join a game with this code")
	private := flag.Bool("private", false, "don't list the hosted game publicly")
	noGame := flag.Bool("nogame", false, "don't touch pt.cfg or start the game")
	joinIP := flag.String("joinip", "", "local address the joiner's game connects to")
	relayURL := flag.String("relay", "", "override relay URL")
	relayKey := flag.String("key", "", "override relay key")
	// Links from the website arrive as the only argument: ptlink://...
	var link []string
	var args []string
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(strings.ToLower(a), "ptlink://") {
			link = append(link, a)
		} else {
			args = append(args, a)
		}
	}
	if len(link) > 0 {
		// Started from a website link: never accept any other options, so a
		// crafted link can't point PTLink at a different relay or settings.
		args = nil
	}
	flag.CommandLine.Parse(args)
	log.SetFlags(log.Ltime)
	registerProtocol()
	if len(link) > 0 {
		if h, priv, code, ok := parseLink(link[0]); ok {
			*host, *private, *join = h, priv, code
		}
	}
	loadConfig()
	if *relayURL != "" {
		RelayURL = *relayURL
	}
	if *relayKey != "" {
		RelayKey = *relayKey
	}

	fmt.Println("=== PTLink: Pocket Tanks online ===")
	fmt.Println("Tip: the Join and Host buttons on the PTLink website now open this app directly.")
	in := bufio.NewReader(os.Stdin)
	if !*host && *join == "" {
		fmt.Println("  H     = host a public game (listed on the PTLink game browser)")
		fmt.Println("  P     = host a private game (friend code only)")
		fmt.Println("  CODE  = paste a game code to join")
		fmt.Print("> ")
		line, _ := in.ReadString('\n')
		line = strings.ToUpper(strings.TrimSpace(line))
		if line == "H" || line == "" {
			*host = true
		} else if line == "P" {
			*host = true
			*private = true
		} else {
			*join = line
		}
	}

	t := newTunnel("j")
	code := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*join), " ", ""))
	if *host {
		t = newTunnel("h")
		code = randCode(4) + "-" + randCode(4)
	} else {
		t.sid = randCode(10)
		t.joinIP = *joinIP
		if t.joinIP == "" {
			t.joinIP = defaultJoinIP()
		}
		t.waitLocal = !*noGame
	}
	if !*host && len(code) == 8 && !strings.Contains(code, "-") {
		code = code[:4] + "-" + code[4:]
	}
	if !*host && !validCode(code) {
		fmt.Println("That code looks wrong. It should look like ABCD-EFGH.")
		fmt.Print("Press Enter to exit.")
		in.ReadString('\n')
		os.Exit(1)
	}
	t.code = code
	t.key = newSessionKey()

	log.Printf("Connecting to relay...")
	r, err := ConnectRelay(RelayURL, RelayKey, "ptlink-"+code, t.onRelay)
	if err != nil {
		fmt.Println("Could not connect to the relay:", err)
		fmt.Print("Press Enter to exit.")
		in.ReadString('\n')
		os.Exit(1)
	}
	t.relay = r
	go t.flushLoop(50 * time.Millisecond)

	gameDir := findGameDir()
	if *host {
		fmt.Println()
		fmt.Println("  Your code:  " + code)
		fmt.Println("  Send it to your friend. Keep this window open while you play.")
		fmt.Println()
		t.onPeer = func() { log.Printf("Friend connected (encrypted).") }
		if !*private {
			lobby := announceGame(code, playerName(gameDir))
			t.onPeer = func() { log.Printf("Friend connected (encrypted)."); lobby.SetStatus("full") }
		} else {
			log.Printf("Private game: only people with the code can join.")
		}
		if !*noGame {
			setCfg(gameDir, "")
			launchGame(gameDir)
		}
		log.Printf("In Pocket Tanks: Network Game -> Create Game.")
		go t.watchPorts()
		if !*noGame {
			go t.lanCatcher()
		}
	} else {
		if !*noGame {
			setCfg(gameDir, t.joinIP)
		}
		log.Printf("Looking for the host...")
		go func() {
			for {
				select {
				case <-t.peerOK:
					return
				case <-r.Done():
					return
				default:
				}
				t.sendHandshake()
				time.Sleep(3 * time.Second)
			}
		}()
		go func() {
			select {
			case <-t.peerOK:
				log.Printf("Connected to host through the relay (encrypted).")
				if !*noGame {
					launchGame(gameDir)
				}
			case <-r.Done():
			}
		}()
	}
	if !*host && !*noGame {
		go t.lanCatcher()
	}
	<-r.Done()
	fmt.Print("Disconnected from relay. Press Enter to exit.")
	in.ReadString('\n')
}
