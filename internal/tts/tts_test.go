package tts

import "testing"

func TestMessageExtractsSpeakText(t *testing.T) {
	got, ok := Message("rc-say: hello chat")
	if !ok || got != "hello chat" {
		t.Fatalf("expected speak text, got %q ok=%v", got, ok)
	}
}

func TestMessageRejectsEmptyOrUnprefixedText(t *testing.T) {
	if _, ok := Message("hello chat"); ok {
		t.Fatal("expected unprefixed text to be ignored")
	}
	if _, ok := Message("rc-say:   "); ok {
		t.Fatal("expected empty tts text to be ignored")
	}
}
