# streamer-remote

Lets Twitch chat trigger keyboard/mouse input on the streamer's own PC in
real time. Single Windows executable, no runtime to install.

## Setup

Just double-click `streamer-remote.exe`. First run walks you through
everything with plain prompts — no terminal knowledge required:

1. It creates `config.yaml` next to itself and offers to open the Twitch
   developer page in your browser to register a free app (Category:
   `Chat Bot`, Client Type: `Public`, OAuth Redirect URL: `http://localhost`).
2. You paste in your channel name and the app's Client ID.
3. Pick "Start" from the menu. First time, it prints a URL and a code —
   open it, log in, done. It remembers you after that (auto-refreshes),
   so this only happens once.

Building from source instead: `go build -ldflags="-s -w" -o streamer-remote.exe ./cmd/streamerctl`.

## Testing without Twitch

Run with `streamer-remote.exe --local`. No Twitch connection is made; type
commands straight into the console instead, e.g. `rc!w` or `rc!w+shift`.
Prefix a line with `@<permission>` to simulate a viewer of that rank, e.g.
`@everyone rc!w` to check what an ordinary viewer can/can't do under
mod-only mode or your blacklist. `--local` can also be left off — the
console test input works even while connected to Twitch, useful for
debugging live.

## Chat command syntax

- `rc!w` — tap the W key
- `rc!w+shift` — combo: hold both, then release (viewers can chain up to `maxComboSize` tokens with `+`)
- `rc!click:left` / `click:right` / `click:middle` — mouse click
- `rc!move:up:50` — move the mouse 50px (direction required, amount optional, capped by `maxMoveStep`)
- `rc!scroll:up` / `scroll:down` — mouse wheel
- `rc!hold:w:1000` — hold a key for an explicit duration in ms (capped by `maxHoldMs`)
- `rc!pause` / `rc!resume` — moderator/broadcaster only; kills or restores the remote instantly

The `rc!` prefix is deliberately unusual so it won't collide with
Nightbot/StreamElements/Moobot-style single-`!` bot commands. Change it in
config if you like.

## Safety

Nothing is blocked by default — the streamer opts into restrictions via
`blacklist.deniedKeys` / `blacklist.deniedCombos` in `config.yaml` (e.g.
block `alt+tab` or `alt+f4` if that's a concern for a given game). Cooldowns
(global and per-viewer) and `modOnlyMode` are also config-driven. Regardless
of config, any moderator can type `rc!pause` to shut it off immediately.

## Logs

Written to `logs/streamer-remote.log` (rotating JSON) and echoed to the
console. Set `logDebug: true` in config (or pass `--debug`) for verbose
output when troubleshooting.

## Updating

The app updates itself — no git required. It checks GitHub Releases on
startup and offers to install if a newer version is out; pick "Check for
updates" from the menu to check on demand, or run `streamer-remote.exe
--update` for a non-interactive update-and-exit (handy for scripting).
It downloads the new `.exe`, swaps it in, and relaunches itself.
`config.yaml` and your saved Twitch login are never touched by an update.

### Cutting a release (maintainers)

Push a tag matching `v*.*.*` (e.g. `git tag v1.1.0 && git push origin
v1.1.0`). `.github/workflows/release.yml` builds the Windows binary with
the version baked in and publishes it as a GitHub Release named after the
tag, with the exe attached as `streamer-remote-windows-amd64.exe` — the
exact name the in-app updater looks for. Every push also runs
`.github/workflows/ci.yml` (vet, test, build).
