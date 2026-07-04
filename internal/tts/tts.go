package tts

import (
	"strings"
)

const Prefix = "rc-say:"
const EventMessage = "text-to-speech"

func Message(text string) (string, bool) {
	body, ok := strings.CutPrefix(text, Prefix)
	if !ok {
		return "", false
	}
	body = strings.TrimSpace(body)
	return body, body != ""
}
