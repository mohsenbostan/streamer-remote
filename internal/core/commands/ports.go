package commands

// InputSink is the outbound port the executor drives to actuate keyboard
// and mouse input on the host. It is the command core's one side-effecting
// dependency on the outside world: everything the core decides to do
// ultimately becomes calls on an InputSink.
//
// internal/input provides the real Windows (SendInput) implementation; the
// composition root injects it. Tests inject a spy instead, so the executor
// can be exercised without moving the real cursor.
type InputSink interface {
	KeyDown(name string) error
	KeyUp(name string) error
	MouseDown(button string) error
	MouseUp(button string) error
	MoveMouseRelative(dx, dy int32) error
	ScrollMouse(delta int32) error
}
