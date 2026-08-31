package changesdesk

import (
	"fmt"
	"strings"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
)

// Policy is policy/review-policy.json: how much human attention each kind of
// change needs. Hard floors live in code, not in this file — no policy can
// make connection or key changes apply without review.
type Policy struct {
	Note               string            `json:"policy_version_note"`
	DefaultClassByKind map[string]string `json:"default_class_by_kind"`
	EscalatePatterns   []string          `json:"escalate_to_two_keys_when_touching"`
	AutoApplyEnabled   bool              `json:"auto_apply_enabled"`
	ExpiryDays         int               `json:"expiry_days"`
	OverlapDays        int               `json:"overlap_days_default"`
	TwoPersonRule      bool              `json:"two_person_rule"`
	RequireOutOfBand   []string          `json:"require_out_of_band_check_for_kinds"`
}

// DefaultPolicy is what a fresh node ships with: everything human-reviewed.
func DefaultPolicy() Policy {
	return Policy{
		Note: "Prototype default: nothing is auto-applied; every change needs a human.",
		DefaultClassByKind: map[string]string{
			KindTrustRulebook:     "review",
			KindUpdateConnections: "review",
		},
		EscalatePatterns: []string{
			"price", "amount", "currency", "iban", "bank", "account",
			"invoice_to", "company_id", "gln", "certificate", "key",
		},
		AutoApplyEnabled: false,
		ExpiryDays:       30,
		OverlapDays:      14,
		TwoPersonRule:    false,
		RequireOutOfBand: []string{KindUpdateConnections},
	}
}

// LoadPolicy reads and validates the policy file, enforcing the hard floors:
//   - auto-apply can never be enabled for connection or key changes
//     (in the prototype: cannot be enabled at all),
//   - connection changes can never be classified below "review".
//
// A policy that tries is rejected at load — editing the file cannot open the
// fraud door.
func LoadPolicy(home *store.Home) (Policy, error) {
	var p Policy
	path := home.File("policy", "review-policy.json")
	if err := store.ReadJSON(path, &p); err != nil {
		return Policy{}, fmt.Errorf("cannot read the review policy: %w", err)
	}
	if p.AutoApplyEnabled {
		return Policy{}, fmt.Errorf("refusing this policy: auto-apply is not available in the prototype — every change needs a human")
	}
	if cls, ok := p.DefaultClassByKind[KindUpdateConnections]; ok && cls != "review" && cls != "two-keys" {
		return Policy{}, fmt.Errorf("refusing this policy: connection changes can never be waved through without review")
	}
	if p.ExpiryDays <= 0 {
		p.ExpiryDays = 30
	}
	if p.OverlapDays <= 0 {
		p.OverlapDays = 14
	}
	return p, nil
}

// Classify assigns the risk class for a proposal and explains why.
func (p Policy) Classify(prop *Proposal) Risk {
	class := p.DefaultClassByKind[prop.Kind]
	if class == "" {
		class = "review"
	}
	because := []string{fmt.Sprintf("default for %s changes", prop.Kind)}
	for _, entry := range prop.WhatChanges {
		lower := strings.ToLower(entry.Item + " " + entry.InPlainWords)
		for _, pattern := range p.EscalatePatterns {
			if strings.Contains(lower, pattern) {
				because = append(because, fmt.Sprintf("%q touches %q", entry.Item, pattern))
				if p.TwoPersonRule {
					class = "two-keys"
				}
			}
		}
	}
	return Risk{Class: class, Because: dedupeStrings(because)}
}

// NeedsOutOfBandCheck says whether approving this kind requires the reviewer
// to confirm through a channel other than the message itself.
func (p Policy) NeedsOutOfBandCheck(kind string) bool {
	for _, k := range p.RequireOutOfBand {
		if k == kind {
			return true
		}
	}
	return false
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
