package input

// Sink is the Windows implementation of the command executor's InputSink
// port. It is a thin adapter over this package's SendInput helpers, holding
// no state: the composition root injects one into the executor.
type Sink struct{}

func NewSink() Sink { return Sink{} }

func (Sink) KeyDown(name string) error            { return KeyDown(name) }
func (Sink) KeyUp(name string) error              { return KeyUp(name) }
func (Sink) MouseDown(button string) error        { return MouseDown(button) }
func (Sink) MouseUp(button string) error          { return MouseUp(button) }
func (Sink) MoveMouseRelative(dx, dy int32) error { return MoveMouseRelative(dx, dy) }
func (Sink) ScrollMouse(delta int32) error        { return ScrollMouse(delta) }
