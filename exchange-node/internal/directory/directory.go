// Package directory is the STUB public-key "phone book". In production this
// is an operated service (Procuros first, an industry body later); in the
// prototype it is one shared JSON file both demo companies can read.
package directory

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
)

type Entry struct {
	Name         string `json:"name"`
	PublicKeyPEM string `json:"public_key_pem"`
}

type Directory struct {
	Path    string
	mu      sync.Mutex
	Entries map[string]Entry `json:"entries"` // keyed by company_id ("GLN ...")
}

func Open(path string) (*Directory, error) {
	d := &Directory{Path: path, Entries: map[string]Entry{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	var file struct {
		Entries map[string]Entry `json:"entries"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("the key directory at %s is not readable: %w", path, err)
	}
	if file.Entries != nil {
		d.Entries = file.Entries
	}
	return d, nil
}

// Publish records (or replaces) a company's public key. Demo-only operation:
// in production, directory writes are the operated service's job.
func (d *Directory) Publish(companyID, name string, pub ed25519.PublicKey) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Re-read so two nodes initialising against the same file don't clobber
	// each other's entries.
	if fresh, err := Open(d.Path); err == nil {
		for k, v := range fresh.Entries {
			if _, mine := d.Entries[k]; !mine {
				d.Entries[k] = v
			}
		}
	}
	d.Entries[companyID] = Entry{Name: name, PublicKeyPEM: sign.PublicPEM(pub)}
	out, _ := json.MarshalIndent(map[string]any{"entries": d.Entries}, "", "  ")
	return os.WriteFile(d.Path, append(out, '\n'), 0o644)
}

// LookUp returns the public key registered for a company id.
func (d *Directory) LookUp(companyID string) (ed25519.PublicKey, error) {
	// Always re-read: another node may have published after we opened.
	fresh, err := Open(d.Path)
	if err != nil {
		return nil, err
	}
	e, ok := fresh.Entries[companyID]
	if !ok {
		return nil, fmt.Errorf("no directory entry for %q", companyID)
	}
	return sign.ParsePublicPEM(e.PublicKeyPEM)
}
