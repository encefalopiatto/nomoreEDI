// Package trust is the READ-ONLY view of the node's live configuration:
// which rulebooks are trusted (and until when), which partners are known,
// and which connection details are on file. Nothing in this package writes.
// The only writer of trusted/ is the changes desk's apply step.
package trust

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
)

// TrustedVersion is one rulebook version the node accepts.
type TrustedVersion struct {
	Version           int    `json:"version"`
	Fingerprint       string `json:"fingerprint"`
	TrustedFrom       string `json:"trusted_from"`
	AlsoAcceptedUntil string `json:"also_accepted_until,omitempty"`
	ApprovedVia       string `json:"approved_via,omitempty"`
}

// RulebookTrust is trusted/rulebooks/<id>/trust.json.
type RulebookTrust struct {
	RulebookID string           `json:"rulebook_id"`
	Accepting  []TrustedVersion `json:"accepting"`
	Retired    []TrustedVersion `json:"retired"`
}

// Partner is trusted/partners/<company_id>.json.
type Partner struct {
	Name           string `json:"name"`
	CompanyID      string `json:"company_id"`
	KeyFingerprint string `json:"key_fingerprint"`
	FirstSeen      string `json:"first_seen"`
	ApprovedVia    string `json:"approved_via,omitempty"`
}

// Status says how a declared rulebook relates to what the node trusts.
type Status string

const (
	StatusTrusted     Status = "trusted"     // fingerprint is accepted
	StatusUnknown     Status = "unknown"     // no trust record for this rulebook id at all
	StatusUpdate      Status = "update"      // version above the highest trusted
	StatusCounterfeit Status = "counterfeit" // known version, different content
	StatusDowngrade   Status = "downgrade"   // version below trusted, not accepted anymore
)

// View reads the trusted/ tree of one node home.
type View struct {
	Home *store.Home
}

func rulebookDirName(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

func (v View) rulebookTrustPath(id string) string {
	return v.Home.File("trusted", "rulebooks", rulebookDirName(id), "trust.json")
}

// RulebookTrust loads the trust record for a rulebook id, if any.
func (v View) RulebookTrust(id string) (RulebookTrust, bool) {
	var rt RulebookTrust
	if err := store.ReadJSON(v.rulebookTrustPath(id), &rt); err != nil {
		return rt, false
	}
	return rt, true
}

// HighestTrusted returns the highest accepted version, if any.
func (rt RulebookTrust) HighestTrusted() (TrustedVersion, bool) {
	var best TrustedVersion
	found := false
	for _, tv := range rt.Accepting {
		if !found || tv.Version > best.Version {
			best, found = tv, true
		}
	}
	return best, found
}

// StatusOf classifies a declared rulebook (id, version, fingerprint) against
// the trust store, honoring overlap windows.
func (v View) StatusOf(id string, version int, fingerprint string, now time.Time) Status {
	rt, ok := v.RulebookTrust(id)
	if !ok {
		return StatusUnknown
	}
	for _, tv := range rt.Accepting {
		if tv.Fingerprint == fingerprint {
			if tv.AlsoAcceptedUntil != "" {
				if until, err := time.Parse("2006-01-02", tv.AlsoAcceptedUntil); err == nil && now.After(until.AddDate(0, 0, 1)) {
					return StatusDowngrade
				}
			}
			return StatusTrusted
		}
		if tv.Version == version {
			return StatusCounterfeit
		}
	}
	for _, tv := range rt.Retired {
		if tv.Fingerprint == fingerprint || tv.Version == version {
			return StatusDowngrade
		}
	}
	if highest, ok := rt.HighestTrusted(); ok && version > highest.Version {
		return StatusUpdate
	}
	// Same or lower version with content we have never trusted: counterfeit.
	return StatusCounterfeit
}

// TrustedRulebookBytes returns the stored canonical bytes of a trusted version.
func (v View) TrustedRulebookBytes(id, fingerprint string) ([]byte, error) {
	dir := v.Home.File("trusted", "rulebooks", rulebookDirName(id))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	short := shortFP(fingerprint)
	for _, e := range entries {
		if strings.Contains(e.Name(), short) && e.Name() != "trust.json" {
			return os.ReadFile(filepath.Join(dir, e.Name()))
		}
	}
	return nil, fmt.Errorf("trusted rulebook %s (%s) is not stored", id, short)
}

// Partner returns the partner record for a company id, if pinned.
func (v View) Partner(companyID string) (Partner, bool) {
	var p Partner
	if err := store.ReadJSON(v.Home.File("trusted", "partners", store.SafeName(companyID)+".json"), &p); err != nil {
		return p, false
	}
	return p, true
}

// Connections returns the connection channels on file for a partner.
func (v View) Connections(companyID string) ([]any, bool) {
	var c struct {
		Channels []any `json:"channels"`
	}
	if err := store.ReadJSON(v.Home.File("trusted", "connections", store.SafeName(companyID)+".json"), &c); err != nil {
		return nil, false
	}
	return c.Channels, true
}

// ListPartners returns every pinned partner.
func (v View) ListPartners() []Partner {
	dir := v.Home.File("trusted", "partners")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Partner
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var p Partner
		if store.ReadJSON(filepath.Join(dir, e.Name()), &p) == nil {
			out = append(out, p)
		}
	}
	return out
}

// ListRulebooks returns every rulebook id with a trust record.
func (v View) ListRulebooks() []RulebookTrust {
	dir := v.Home.File("trusted", "rulebooks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []RulebookTrust
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var rt RulebookTrust
		if store.ReadJSON(filepath.Join(dir, e.Name(), "trust.json"), &rt) == nil {
			out = append(out, rt)
		}
	}
	return out
}

func shortFP(fp string) string {
	fp = strings.TrimPrefix(fp, "sm0-")
	if len(fp) > 12 {
		return fp[:12]
	}
	return fp
}

// ShortFP is the 12-character display form of a fingerprint.
func ShortFP(fp string) string { return shortFP(fp) }
