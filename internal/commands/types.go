package commands

// Kind identifies what an Action does when executed.
type Kind int

const (
	KindKey Kind = iota
	KindClick
	KindMove
	KindScroll
)

// Action is one parsed step of a viewer's command, ready for the executor.
// A combo (e.g. "shift+w") parses into multiple Actions that run together.
type Action struct {
	Kind   Kind
	Name   string // key name, mouse button name, or move/scroll direction
	Amount int32  // pixels for move, wheel units for scroll
	HoldMs int    // explicit hold override in ms; 0 means "use the default"
}

// ChatMessage is a source-agnostic chat line: it can come from Twitch
// EventSub or from the local console test harness.
type ChatMessage struct {
	Username   string
	Permission Permission
	Text       string
}
