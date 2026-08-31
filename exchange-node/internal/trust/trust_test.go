package trust

import (
	"os"
	"testing"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
)

// Downgrades and counterfeits must be recognized from the trust record.
func TestStatusOf(t *testing.T) {
	home := store.Open(t.TempDir())
	if err := home.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	v := View{Home: home}
	rt := RulebookTrust{
		RulebookID: "rb.test",
		Accepting: []TrustedVersion{
			{Version: 3, Fingerprint: "sm0-old", TrustedFrom: "2026-01-01", AlsoAcceptedUntil: "2026-02-01"},
			{Version: 4, Fingerprint: "sm0-new", TrustedFrom: "2026-02-01"},
		},
	}
	if err := os.MkdirAll(home.File("trusted", "rulebooks", "rb.test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(home.File("trusted", "rulebooks", "rb.test", "trust.json"), rt); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	if got := v.StatusOf("rb.test", 4, "sm0-new", now); got != StatusTrusted {
		t.Fatalf("current version must be trusted, got %s", got)
	}
	if got := v.StatusOf("rb.test", 3, "sm0-old", now); got != StatusDowngrade {
		t.Fatalf("an old version outside its overlap window is a downgrade, got %s", got)
	}
	if got := v.StatusOf("rb.test", 4, "sm0-DIFFERENT", now); got != StatusCounterfeit {
		t.Fatalf("a known version with different content is a counterfeit, got %s", got)
	}
	if got := v.StatusOf("rb.test", 5, "sm0-next", now); got != StatusUpdate {
		t.Fatalf("a higher version is an update proposal, got %s", got)
	}
	if got := v.StatusOf("rb.unknown", 1, "sm0-x", now); got != StatusUnknown {
		t.Fatalf("an unknown rulebook id is unknown, got %s", got)
	}
}
