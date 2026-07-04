// Package input simulates keyboard and mouse events on Windows via the
// SendInput syscall. It performs no permission or safety checks — callers
// (the commands package) are responsible for validating actions before
// they reach here.
package input

import (
	"fmt"
	"time"
)

// KeyDown/KeyUp resolve a key name to its virtual-key code and inject the
// corresponding event. They are split out (rather than a single "tap")
// so a combo can hold modifiers across multiple keys.
func KeyDown(name string) error { return sendKey(name, false) }
func KeyUp(name string) error   { return sendKey(name, true) }

func sendKey(name string, up bool) error {
	vk, ok := vkCodes[name]
	if !ok {
		return fmt.Errorf("input: unknown key %q", name)
	}
	if _, err := sendInputs([]rawInput{keyInput(vk, up)}); err != nil {
		return fmt.Errorf("input: SendInput key %q: %w", name, err)
	}
	return nil
}

// TapKey presses and releases a key, holding it for holdFor to give the
// target application a chance to register the keystroke.
func TapKey(name string, holdFor time.Duration) error {
	if err := KeyDown(name); err != nil {
		return err
	}
	time.Sleep(holdFor)
	return KeyUp(name)
}

var mouseButtonFlags = map[string][2]uint32{
	"left":   {mouseEventFLeftDown, mouseEventFLeftUp},
	"right":  {mouseEventFRightDown, mouseEventFRightUp},
	"middle": {mouseEventFMiddleDown, mouseEventFMiddleUp},
}

func IsMouseButtonKnown(name string) bool {
	_, ok := mouseButtonFlags[name]
	return ok
}

func MouseDown(button string) error { return sendMouseButton(button, true) }
func MouseUp(button string) error   { return sendMouseButton(button, false) }

func sendMouseButton(button string, down bool) error {
	flags, ok := mouseButtonFlags[button]
	if !ok {
		return fmt.Errorf("input: unknown mouse button %q", button)
	}
	flag := flags[1]
	if down {
		flag = flags[0]
	}
	if _, err := sendInputs([]rawInput{mouseInputEvent(flag, 0, 0, 0)}); err != nil {
		return fmt.Errorf("input: SendInput mouse %q: %w", button, err)
	}
	return nil
}

// ClickMouse presses and releases a mouse button, holding it for holdFor.
func ClickMouse(button string, holdFor time.Duration) error {
	if err := MouseDown(button); err != nil {
		return err
	}
	time.Sleep(holdFor)
	return MouseUp(button)
}

// MoveMouseRelative moves the cursor by (dx, dy) pixels from its current position.
func MoveMouseRelative(dx, dy int32) error {
	if _, err := sendInputs([]rawInput{mouseInputEvent(mouseEventFMove, dx, dy, 0)}); err != nil {
		return fmt.Errorf("input: SendInput move: %w", err)
	}
	return nil
}

// ScrollMouse spins the wheel. delta is in multiples of the OS wheel unit
// (120); positive scrolls up/away from the user.
func ScrollMouse(delta int32) error {
	if _, err := sendInputs([]rawInput{mouseInputEvent(mouseEventFWheel, 0, 0, uint32(delta))}); err != nil {
		return fmt.Errorf("input: SendInput scroll: %w", err)
	}
	return nil
}
