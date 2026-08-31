package audit

import (
	"os"
	"strings"
	"testing"
)

// The chain must verify when intact and break loudly when any line is edited.
func TestChainVerifiesAndDetectsTampering(t *testing.T) {
	path := t.TempDir() + "/audit.jsonl"
	l := Open(path)
	if err := l.Append("first", map[string]any{"detail": "a"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append("second", map[string]any{"detail": "b"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Verify(); err != nil {
		t.Fatalf("an untouched chain must verify: %v", err)
	}
	b, _ := os.ReadFile(path)
	tampered := strings.Replace(string(b), `"detail":"a"`, `"detail":"X"`, 1)
	if tampered == string(b) {
		t.Fatal("test setup: nothing replaced")
	}
	os.WriteFile(path, []byte(tampered), 0o644)
	if err := Open(path).Verify(); err == nil {
		t.Fatal("an edited line must break the chain")
	}
}
