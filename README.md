# PTLink

Play **Pocket Tanks** online with a friend using a short code. No IP addresses
shared, no port forwarding, no accounts for players.

> Unofficial fan project. Not affiliated with or endorsed by Blitwise Productions.
> Pocket Tanks is a trademark of Blitwise Productions. Each player needs their own copy of the game.

## How to play
1. Download `PTLink.zip` from [Releases](https://github.com/vermosi/ptlink/releases/latest), unzip, and double-click `PTLink.exe` (don't run it as administrator).
2. **Host:** type `H`. PTLink shows a code like `ABCD-EFGH`; send it to your friend.
   **Join:** paste the code.
3. PTLink turns on Pocket Tanks' built-in IP-to-IP mode and starts the game.
   - Host: Network Game → IP-to-IP Internet Game → create a game.
   - Joiner: Network Game → IP-to-IP → join `127.0.0.2`.
4. Keep the PTLink window open while you play.

## How it works
Pocket Tanks already has a hidden IP-to-IP mode (`networkIPSupport` in `pt.cfg`).
PTLink pretends to be the host on the joiner's own PC (`127.0.0.2`) and carries the
game's TCP/UDP traffic through a **Supabase Realtime broadcast channel** named after the code.
Each player only ever talks to the relay, so neither sees the other's IP.

## Run your own relay
Any Supabase project works (Lovable Cloud uses Supabase). Put `ptlink.json` next to the exe:
```json
{ "relay_url": "https://YOUR-PROJECT.supabase.co", "relay_key": "YOUR_ANON_OR_PUBLISHABLE_KEY" }
```

## Build
```
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o PTLink.exe .
```
Standard library only, no dependencies.

## License
MIT
