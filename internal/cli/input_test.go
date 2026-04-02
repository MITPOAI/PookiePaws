package cli

import "testing"

func TestPromptBufferApplyChunkHandlesBackspace(t *testing.T) {
	buffer := promptBuffer{}

	if action := buffer.ApplyChunk([]byte("abc")); action != promptActionContinue {
		t.Fatalf("expected continue action, got %v", action)
	}
	if action := buffer.ApplyChunk([]byte{0x7f}); action != promptActionContinue {
		t.Fatalf("expected continue action for backspace, got %v", action)
	}
	if action := buffer.ApplyChunk([]byte("d\r")); action != promptActionSubmit {
		t.Fatalf("expected submit action, got %v", action)
	}
	if got := buffer.String(); got != "abd" {
		t.Fatalf("expected buffer to be %q, got %q", "abd", got)
	}
}

func TestPromptBufferApplyChunkEscCancels(t *testing.T) {
	buffer := promptBuffer{}

	if action := buffer.ApplyChunk([]byte{0x1b}); action != promptActionCancel {
		t.Fatalf("expected cancel action, got %v", action)
	}
}

func TestPromptBufferApplyChunkIgnoresArrowKeys(t *testing.T) {
	buffer := promptBuffer{}
	buffer.ApplyChunk([]byte("sk-"))

	if action := buffer.ApplyChunk([]byte{0x1b, '[', 'A'}); action != promptActionContinue {
		t.Fatalf("expected continue action, got %v", action)
	}
	if got := buffer.String(); got != "sk-" {
		t.Fatalf("expected buffer to stay %q, got %q", "sk-", got)
	}
}
