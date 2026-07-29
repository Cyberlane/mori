package diagnostic

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMessageRemovesPathOperands(t *testing.T) {
	t.Parallel()

	privatePath := "/private/workstation/project/source.go"
	message := Message(&os.PathError{
		Op:   "open",
		Path: privatePath,
		Err:  os.ErrNotExist,
	})
	if strings.Contains(message, privatePath) {
		t.Fatalf("message %q contains private path", message)
	}
	if !strings.Contains(message, "open") || !strings.Contains(message, "does not exist") {
		t.Fatalf("message = %q, want operation and cause", message)
	}
}

func TestMessagePreservesUnstructuredErrors(t *testing.T) {
	t.Parallel()

	if got := Message(errors.New("parser failed")); got != "parser failed" {
		t.Fatalf("Message = %q", got)
	}
}
