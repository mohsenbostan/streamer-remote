# Architecture

streamer-remote is a small Windows desktop app with a clear shape:
**many chat inputs → one command core → one input output**, plus a local
web dashboard to drive it. The layout is a pragmatic blend of
**hexagonal (ports & adapters)** for that core and **idiomatic Go
package layout** for everything around it.

The guiding idea: the part that decides *what to do* (parse a chat
command, check permissions, enforce the blacklist, run a sequence) is
isolated and depends on nothing platform-specific. The parts that talk
to the outside world (Twitch, Kick, the Windows input API, a browser)
are adapters around it.

## The hexagon: `internal/core/commands`

The command core owns the domain and knows nothing about who is calling
it or how input is actually delivered:

- `types.go` — `ChatMessage`, `Action`, `Step`, `Kind` (the vocabulary)
- `parser.go` — chat text → validated `Step`s
- `permission.go` — badge tags → a `Permission` rank
- `blacklist.go` — streamer-configured denials
- `dispatcher.go` — the trust boundary: permissions, cooldowns,
  blacklist, reward gating, pause state
- `executor.go` — runs sequences on a serialized worker
- `rewards.go` — Channel Points redemption handling
- `ports.go` — the outbound port(s) the core drives

A `ChatMessage` is source-agnostic on purpose: Twitch, Kick, and the
dashboard's quick-test box all build the same struct, so the dispatcher
treats every platform identically. That is why multistreaming to Twitch
and Kick at once "just works" — both are only producers of
`ChatMessage`.

### The one inverted port: `InputSink`

The executor's single side effect — pressing keys and moving the mouse —
is inverted behind the `InputSink` interface (`ports.go`). The core
depends on the interface; `internal/input` provides the real Windows
(`SendInput`) implementation; the composition root injects it. Tests
inject a no-op sink, so the dispatcher and executor can be exercised
without moving the real cursor.

TTS and Twitch reward-status callbacks are the other outbound effects;
they already live at the composition root (`internal/app`) and reach the
core only as injected callbacks, not as core dependencies.

## Adapters (grouped by external system, idiomatic Go)

Each package wraps one external system. Inbound adapters produce
`ChatMessage`s; outbound adapters satisfy a core port.

- `internal/twitch` — EventSub chat + redemptions (inbound) and Helix
  reward management / redemption status (outbound)
- `internal/twitchauth` — Twitch OAuth device-code flow + token cache
- `internal/kick` — Kick public Pusher chat (inbound); no auth needed
- `internal/input` — Windows `SendInput`; implements `InputSink`
- `internal/tts` — spoken-message detection
- `internal/webui` — the dashboard: HTTP/SSE API + embedded frontend
  (a driving adapter for the human)

## Composition root: `internal/app`

`internal/app` wires the core to its adapters and supervises the
long-running goroutines (restart-with-backoff on panic/exit). It is the
only place that knows about both the core and every adapter at once.
`cmd/streamerctl/main.go` is a thin entrypoint that builds the config,
logger, core, and dashboard, then hands off.

## Support packages (flat, no ceremony)

Plain leaf utilities with no domain knowledge, imported where needed:
`config`, `logging`, `update`, `tray`, `browser`, `msgbox`, `backoff`.

## Dependency rules

1. `internal/core/commands` imports no adapter and no platform package.
   Its only outward reach is through a port interface it defines itself.
   (One benign exception: the parser links `internal/input`'s pure
   mouse-button *name* lookup — a stateless query, not a side effect.)
2. Adapters may import the core (to build `ChatMessage`s or implement a
   port). The core never imports an adapter.
3. Only `internal/app` (and `cmd`) may import both the core and the
   adapters, to wire them together.
4. `config` is a shared kernel: the core reads limits from it directly
   rather than through a port, a deliberate simplification for a
   single-binary app.

## Adding a new chat platform

The Kick integration is the worked example: add an adapter package that
connects to the platform and emits `commands.ChatMessage`, add a
`Start<Platform>` wiring function in `internal/app`, and surface it in
the dashboard. The core does not change — which is the whole point of
the port boundary.
