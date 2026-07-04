package input

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 SendInput ABI. Not exposed by golang.org/x/sys/windows, so it is
// reproduced here from the Win32 headers. Layout is load-bearing: the union
// inside INPUT is sized/aligned to its largest member (mouseInput on amd64),
// so keybdInput is placed in a same-sized byte buffer rather than as its own
// field to keep the union honest on both 386 and amd64.
const (
	inputMouse    uint32 = 0
	inputKeyboard uint32 = 1

	keyEventFExtendedKey uint32 = 0x0001
	keyEventFKeyUp       uint32 = 0x0002

	mouseEventFMove       uint32 = 0x0001
	mouseEventFLeftDown   uint32 = 0x0002
	mouseEventFLeftUp     uint32 = 0x0004
	mouseEventFRightDown  uint32 = 0x0008
	mouseEventFRightUp    uint32 = 0x0010
	mouseEventFMiddleDown uint32 = 0x0020
	mouseEventFMiddleUp   uint32 = 0x0040
	mouseEventFWheel      uint32 = 0x0800
)

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// rawInput mirrors Win32's INPUT struct. The union is represented as a
// fixed byte array sized to the largest member (mouseInput) so the type
// works regardless of which variant is populated.
type rawInput struct {
	inputType uint32
	_         uint32   // alignment padding to match the compiler-inserted gap before the union on amd64
	union     [32]byte // sized/aligned for mouseInput, the largest union member
}

var (
	user32        = windows.NewLazySystemDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

func sendInputs(inputs []rawInput) (uint32, error) {
	if len(inputs) == 0 {
		return 0, nil
	}
	sizeOfInput := unsafe.Sizeof(rawInput{})
	ret, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		sizeOfInput,
	)
	sent := uint32(ret)
	if sent != uint32(len(inputs)) {
		return sent, err
	}
	return sent, nil
}

func keyInput(vk uint16, keyUp bool) rawInput {
	var flags uint32
	if keyUp {
		flags |= keyEventFKeyUp
	}
	ki := keybdInput{wVk: vk, dwFlags: flags}
	var raw rawInput
	raw.inputType = inputKeyboard
	*(*keybdInput)(unsafe.Pointer(&raw.union[0])) = ki
	return raw
}

func mouseInputEvent(flags uint32, dx, dy int32, data uint32) rawInput {
	mi := mouseInput{dx: dx, dy: dy, mouseData: data, dwFlags: flags}
	var raw rawInput
	raw.inputType = inputMouse
	*(*mouseInput)(unsafe.Pointer(&raw.union[0])) = mi
	return raw
}
