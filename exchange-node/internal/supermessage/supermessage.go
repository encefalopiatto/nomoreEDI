// Package supermessage handles the supermessage file format itself:
// parsing, canonical bytes (RFC 8785), fingerprints, and the exact bytes
// that get signed. Everything else in the node builds on this.
package supermessage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gowebpki/jcs"
)

// Doc is one parsed supermessage. Raw keeps the exact bytes as received
// (the archive stores these); M is the decoded tree with numbers kept as
// json.Number so re-marshalling never changes a digit.
type Doc struct {
	Raw []byte
	M   map[string]any
}

func Parse(b []byte) (*Doc, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("this file is not readable JSON: %w", err)
	}
	return &Doc{Raw: b, M: m}, nil
}

// Section returns a named top-level object section.
func (d *Doc) Section(name string) (map[string]any, bool) {
	v, ok := d.M[name].(map[string]any)
	return v, ok
}

// GetString walks a dotted path of object keys and returns a string value.
func GetString(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[p]
	}
	s, _ := cur.(string)
	return s
}

// DeepCopy copies a decoded JSON tree (maps, slices, scalars, json.Number).
func DeepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = DeepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = DeepCopy(val)
		}
		return out
	default:
		return v
	}
}

// Canonical marshals a value and applies RFC 8785 canonicalization,
// producing the one true byte form used for fingerprints and signatures.
func Canonical(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(plain)
}

// blankSignatureValue sets sig.value to "" if the object has a signature-like
// block under the given key, returning whether it was present.
func blankSignatureValue(m map[string]any, key string) {
	if sig, ok := m[key].(map[string]any); ok {
		if _, has := sig["value"]; has {
			sig["value"] = ""
		}
	}
}

// RulebookSigningBytes returns the canonical bytes of a rulebook section
// with its publisher signature value blanked — the bytes the publisher signs
// and the bytes the fingerprint is computed over.
func RulebookSigningBytes(rulebook map[string]any) ([]byte, error) {
	cp := DeepCopy(rulebook).(map[string]any)
	blankSignatureValue(cp, "publisher_signature")
	return Canonical(cp)
}

// Fingerprint computes the rulebook fingerprint: "sm0-" + hex(sha256(canonical
// rulebook with publisher signature value blanked)).
func Fingerprint(rulebook map[string]any) (string, error) {
	b, err := RulebookSigningBytes(rulebook)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sm0-" + hex.EncodeToString(sum[:]), nil
}

// FileSigningBytes returns the canonical bytes of a whole supermessage with
// every signature value blanked — the bytes the sender signs.
func FileSigningBytes(m map[string]any) ([]byte, error) {
	cp := DeepCopy(m).(map[string]any)
	if about, ok := cp["about"].(map[string]any); ok {
		blankSignatureValue(about, "signature")
	}
	if rb, ok := cp["rulebook"].(map[string]any); ok {
		blankSignatureValue(rb, "publisher_signature")
	}
	return Canonical(cp)
}

// Normalize converts json.Number values into int64 (when whole) or float64,
// recursively. The checker and the screens work on normalized trees; the
// archive and all signatures work on the raw bytes.
func Normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = Normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = Normalize(val)
		}
		return out
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64()
		return f
	default:
		return v
	}
}

// MarshalPretty renders a tree as readable indented JSON (for drafts,
// review records and other files humans open in an editor).
func MarshalPretty(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return append(b, '\n')
}
