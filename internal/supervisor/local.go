package supervisor

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"strings"

	"streamer-remote/internal/commands"
)

// runLocalConsole lets a streamer test binds without Twitch at all: every
// line typed into the console is treated as a chat message. It runs
// alongside Twitch mode too, which is handy for testing live.
//
// console must be the single shared reader over the process's stdin (see
// Options.Console): the interactive setup wizard and menu in main also
// read from stdin, and two independent buffered readers over the same
// file descriptor can silently steal each other's bytes.
//
// Plain text is treated as coming from the broadcaster (the console is
// already local-machine-trusted). Prefixing a line with "@<permission> "
// simulates a chatter of that rank, e.g. "@everyone rc!w" to test
// mod-only mode or the blacklist as an ordinary viewer would see it.
func runLocalConsole(ctx context.Context, logger *slog.Logger, dispatcher *commands.Dispatcher, console *bufio.Reader) {
	logger.Info("local test console ready: type a command, or '@<permission> <command>' to simulate a viewer rank")

	lines := make(chan string)
	go func() {
		for {
			line, err := console.ReadString('\n')
			if line != "" {
				lines <- strings.TrimRight(line, "\r\n")
			}
			if err != nil {
				if err != io.EOF {
					logger.Debug("local console: read error", "error", err)
				}
				close(lines)
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				// stdin closed (e.g. piped input ended): nothing left to
				// read. This is not a crash, so don't let the supervisor
				// restart-loop us; just idle until shutdown.
				logger.Info("local console: stdin closed, no more test input will be read")
				<-ctx.Done()
				return
			}
			handleLocalLine(logger, dispatcher, line)
		}
	}
}

func handleLocalLine(logger *slog.Logger, dispatcher *commands.Dispatcher, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	permission := commands.Broadcaster
	text := line
	if strings.HasPrefix(line, "@") {
		parts := strings.SplitN(line[1:], " ", 2)
		if len(parts) != 2 {
			logger.Warn("local console: expected '@<permission> <command>'", "input", line)
			return
		}
		perm, ok := commands.ParsePermission(parts[0])
		if !ok {
			logger.Warn("local console: unknown permission", "permission", parts[0])
			return
		}
		permission, text = perm, parts[1]
	}

	dispatcher.Handle(commands.ChatMessage{
		Username:   "local-console",
		Permission: permission,
		Text:       text,
	})
}
