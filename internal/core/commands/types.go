package commands

// Kind identifies what an Action does when executed.
type Kind int

const (
	KindKey Kind = iota
	KindClick
	KindMove
	KindScroll
)

// Action is one parsed piece of a combo, ready for the executor. A combo
// (e.g. "shift+w") parses into multiple Actions that run simultaneously —
// pressed together, held, released together.
type Action struct {
	Kind    Kind
	Name    string // key name, mouse button name, move/scroll direction, or "xy" for an explicit dx,dy move
	Amount  int32  // pixels for move, wheel units for scroll, or dx for an "xy" move
	Amount2 int32  // dy for an "xy" move; unused otherwise
	HoldMs  int    // explicit hold override in ms; 0 means "use the default"
}

// Step is one entry in a sequence: either a combo to press-hold-release
// (Actions non-empty) or a pure delay (Actions empty, just sleeps for
// HoldMs). A chat command like "alt+f10,wait:800,enter" parses into three
// Steps, run one after another by the executor.
type Step struct {
	Actions []Action
	HoldMs  int
}

// ChatMessage is a source-agnostic chat line: it can come from Twitch
// EventSub or from the local console test harness.
type ChatMessage struct {
	Username   string
	Permission Permission
	Text       string
}
