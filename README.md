# streamer-remote

Lets Twitch chat trigger keyboard/mouse input on the streamer's own PC in
real time. Single Windows executable, no runtime to install, configured
and monitored through a local web dashboard.

## Setup

Just double-click `streamer-remote.exe`. No console window, no terminal
knowledge needed — it puts a small icon in the system tray and opens a
dashboard in your browser at `http://127.0.0.1:47829`:

1. **Overview** tab offers to open the Twitch developer page and asks for
   your channel name + the app's Client ID (Category: `Chat Bot`, Client
   Type: `Public`, OAuth Redirect URL: `http://localhost`).
2. Click **Connect** — first time, it shows a code and a link; open it,
   log in, done. It remembers you after that (auto-refreshes), so this
   only happens once, even across updates.
3. Use the tray icon (Open Dashboard / Pause / Quit) any time the browser
   tab is closed.

Building from source: build the frontend once (`cd web && npm install &&
npm run build`), then `go build -ldflags="-s -w" -o streamer-remote.exe
./cmd/streamerctl`. Add `-H=windowsgui` to the ldflags to suppress the
console window, matching the shipped release build.

## The dashboard

- **Overview** — connect/reconnect Twitch, and a "quick test" box to fire
  commands as if they came from chat, at any permission level, without
  needing to be live.
- **Live monitor** — every dispatched command, blocked attempt, redemption,
  and connection event streamed in real time.
- **Rewards** — set up channel-points-only actions (see below).
- **Settings** — prefix, cooldowns, mod-only mode, the blacklist, and
  checking/installing updates. Changes apply immediately, no restart.

## Testing without Twitch

Run with `streamer-remote.exe --local`: no Twitch connection is made, but
the dashboard still comes up, and its Overview tab's "quick test" box
works exactly the same — pick a permission level (Everyone through
Broadcaster) and send a command, e.g. `rc!w` or `rc!w+shift`, to check
what an ordinary viewer can/can't do under mod-only mode or your
blacklist. The quick test box works in normal (Twitch-connected) mode too.

## Chat command syntax

- `rc!w` — tap the W key
- `rc!w+shift` — combo: hold both, then release (viewers can chain up to `maxComboSize` tokens with `+`)
- `rc!click:left` / `click:right` / `click:middle` — mouse click
- `rc!move:up:50` — move the mouse 50px (direction required, amount optional, capped by `maxMoveStep`)
- `rc!scroll:up` / `scroll:down` — mouse wheel
- `rc!hold:w:1000` — hold a key for an explicit duration in ms (capped by `maxHoldMs`)
- `rc!pause` / `rc!resume` — moderator/broadcaster only; kills or restores the remote instantly (same as the dashboard's Active/Paused switch)

The `rc!` prefix is deliberately unusual so it won't collide with
Nightbot/StreamElements/Moobot-style single-`!` bot commands. Change it in
the Settings tab if you like.

## Safety

Nothing is blocked by default — the streamer opts into restrictions via
the blacklist in the Settings tab (e.g. block `alt+tab` or `alt+f4` if
that's a concern for a given game). Cooldowns (global and per-viewer) and
mod-only mode are also there. Regardless of settings, any moderator can
type `rc!pause` or use the dashboard switch to shut it off immediately.

Moderators and the broadcaster are exempt from cooldowns, the blacklist,
and channel-points-only gating when they type a command themselves — a
human mod present in chat is trusted with everything, no limits.

## Channel-points-only actions

Some actions are more fun (or more dangerous) as a paid, deliberate
redemption rather than something anyone can spam in chat — locking the
screen, `alt+f4`. On the Rewards tab, click Add: give it the action (same
syntax as a chat command, e.g. `alt+f4` or `lwin`), a reward title, and a
point cost, and the app creates the Channel Points reward on Twitch
itself — no Twitch dashboard visit needed. From then on, that action can
only be triggered by redeeming the reward; typing it in chat is rejected
for ordinary viewers (mods can still type it directly). If the remote is
paused or the action later gets blacklisted, redemptions are automatically
refunded instead of silently doing nothing.

Requires the channel to be a Twitch Affiliate or Partner (a Twitch
requirement for Channel Points, not something this app can work around).

## Logs

Written to `logs/streamer-remote.log` (rotating JSON) and streamed live to
the dashboard's Live Monitor tab. Toggle "Verbose logging" in Settings (or
pass `--debug`) to see every rejected/dropped command too.

## Updating

The app updates itself — no git required. Check from the Settings tab, or
run `streamer-remote.exe --update` for a non-interactive update-and-exit
(handy for scripting). It downloads the new `.exe`, swaps it in, and
relaunches itself. `config.yaml` and your saved Twitch login are never
touched by an update.

### Cutting a release (maintainers)

Push a tag matching `v*.*.*` (e.g. `git tag v1.1.0 && git push origin
v1.1.0`). `.github/workflows/release.yml` builds the frontend, then the
Windows binary with the version baked in, and publishes it as a GitHub
Release named after the tag, with the exe attached as
`streamer-remote-windows-amd64.exe` — the exact name the in-app updater
looks for. Every push also runs `.github/workflows/ci.yml` (frontend
build, vet, race-tested tests, build).
