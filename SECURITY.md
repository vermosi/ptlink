# Security

PTLink connects two Pocket Tanks players through a public relay. This is what it does to keep that safe.

## Protections
- **Only the game is reachable.** PTLink only forwards traffic to the ports Pocket Tanks itself opened. The other player can't use PTLink to reach any other program on your computer.
- **End-to-end encryption.** After a short handshake (X25519 key exchange, keys derived with HKDF-SHA256, salted with the game code), all game traffic is encrypted and authenticated with AES-256-GCM. The relay and anyone else listening on the channel can't read or change it.
- **One opponent per game.** The first player to complete the handshake gets the game; later attempts with the same code are ignored.
- **Website links are restricted.** When started from a `ptlink://` link, PTLink ignores every other option, and game codes must match the `ABCD-EFGH` format.
- **Resource limits.** Packet sizes, queued data, connections and ports are capped so a misbehaving peer can't exhaust memory.
- **No administrator rights needed** and none should be used.
- **Reproducible downloads.** Releases are built by GitHub Actions from this code, with SHA-256 checksums.

## Known limits
- **Public games:** the code is listed publicly, so anyone can take the open seat. Use a private game (P) to play only with a friend.
- **Your IP address** is only seen by the relay (Supabase), never by the other player.
- The relay's public key is in the app by design. It only allows Realtime broadcast and presence on PTLink channels.
- Binaries are not code-signed yet (Windows SmartScreen / macOS Gatekeeper warnings).

## Reporting
Please open a private security advisory on GitHub (Security → Report a vulnerability) rather than a public issue.
