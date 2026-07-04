package supervisor

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
)

// TestSharedConsoleReaderPreservesRemainingInput guards against the bug
// where an interactive wizard/menu and the local test console each wrap
// os.Stdin in their own buffered reader: whichever reads first can starve
// the other of bytes it needed. Passing one shared *bufio.Reader through
// must mean lines consumed "by the wizard" are gone, but everything typed
// afterward still reaches the local console intact.
func TestSharedConsoleReaderPreservesRemainingInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("wizard-answer-1\nwizard-answer-2\nrc!w\n"))

	// Simulate the wizard/menu consuming its two lines first.
	for i := 0; i < 2; i++ {
		if _, err := reader.ReadString('\n'); err != nil {
			t.Fatalf("failed to consume simulated wizard line %d: %v", i, err)
		}
	}

	cfg := &config.Config{Prefix: "rc!", MaxComboSize: 3, TapHoldMs: 40, MaxHoldMs: 3000, MaxMoveStep: 300}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := commands.NewExecutor(logger, 10)
	dispatcher := commands.NewDispatcher(cfg, logger, executor)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		runLocalConsole(ctx, logger, dispatcher, reader)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for executor.QueueLen() == 0 {
		select {
		case <-deadline:
			t.Fatal("expected the remaining 'rc!w' line to reach the dispatcher, but it never did")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	<-done
}
