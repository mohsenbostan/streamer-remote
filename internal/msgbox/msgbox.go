// Package msgbox shows a native Windows message box. Used only for fatal
// startup errors: the release build has no console window, so without
// this a non-technical streamer would see the app silently do nothing.
package msgbox

import (
	"golang.org/x/sys/windows"
)

const (
	iconError uint32 = 0x10
	okButton  uint32 = 0x0
)

// Error shows a blocking native error dialog with title and message.
func Error(title, message string) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	msgPtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, msgPtr, titlePtr, okButton|iconError)
}
