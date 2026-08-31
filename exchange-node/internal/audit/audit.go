// Package audit is the node's tamper-evident diary. Every decision, every
// applied change, and every refusal is one line in an append-only file where
// each line carries the hash of the one before it — editing any past line
// breaks the chain from that point on.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
)

type Log struct {
	Path string
	Now  func() time.Time
}

func Open(path string) *Log { return &Log{Path: path, Now: time.Now} }

// Append writes one audit record. The record map is extended with the
// timestamp and the two chain hashes.
func (l *Log) Append(event string, record map[string]any) error {
	if record == nil {
		record = map[string]any{}
	}
	record["event"] = event
	record["at"] = l.Now().UTC().Format(time.RFC3339)
	record["previous_record_hash"] = l.lastHash()
	record["this_record_hash"] = ""
	canon, err := supermessage.Canonical(record)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canon)
	record["this_record_hash"] = "sha256:" + hex.EncodeToString(sum[:])

	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func (l *Log) lastHash() string {
	records, err := l.readAll()
	if err != nil || len(records) == 0 {
		return "sha256:genesis"
	}
	h, _ := records[len(records)-1]["this_record_hash"].(string)
	return h
}

func (l *Log) readAll() ([]map[string]any, error) {
	b, err := os.ReadFile(l.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	dec := json.NewDecoder(newLineReader(b))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Verify walks the whole chain and reports the first broken link, if any.
func (l *Log) Verify() error {
	records, err := l.readAll()
	if err != nil {
		return err
	}
	prev := "sha256:genesis"
	for i, rec := range records {
		gotPrev, _ := rec["previous_record_hash"].(string)
		if gotPrev != prev {
			return fmt.Errorf("audit line %d does not chain to the line before it", i+1)
		}
		claimed, _ := rec["this_record_hash"].(string)
		cp := map[string]any{}
		for k, v := range rec {
			cp[k] = v
		}
		cp["this_record_hash"] = ""
		canon, err := supermessage.Canonical(cp)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(canon)
		if claimed != "sha256:"+hex.EncodeToString(sum[:]) {
			return fmt.Errorf("audit line %d has been altered (hash does not match its content)", i+1)
		}
		prev = claimed
	}
	return nil
}

// Read returns every audit record, oldest first.
func (l *Log) Read() []map[string]any {
	records, _ := l.readAll()
	return records
}

// newLineReader lets json.Decoder stream concatenated JSON lines.
type lineReader struct {
	b   []byte
	pos int
}

func newLineReader(b []byte) *lineReader { return &lineReader{b: b} }

func (r *lineReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}
