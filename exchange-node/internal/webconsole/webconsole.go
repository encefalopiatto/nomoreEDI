// Package webconsole serves the node's review screen: one local web page
// showing the inbox, the review queue, quarantine, trusted rulebooks and
// drafts, with Approve/Reject and response forms. The page, the CLI and the
// tests all call the same Node methods; this file is just HTTP around them.
package webconsole

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/engine"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/respond"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
)

//go:embed console.html
var consoleHTML embed.FS

// Server wraps one node with a mutex (the poller, the page and the demo all
// drive the same node) and the HTTP surface.
type Server struct {
	Node *engine.Node
	Mu   sync.Mutex
}

// WithLock runs one operation against the node under the server's lock.
func (s *Server) WithLock(fn func(n *engine.Node)) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	fn(s.Node)
}

// State gathers everything the console shows, from the files on disk.
func State(n *engine.Node) map[string]any {
	var inbox []engine.InboxRecord
	for _, name := range n.Home.ListDir("inbox") {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var rec engine.InboxRecord
		if store.ReadJSON(n.Home.File("inbox", name), &rec) == nil {
			inbox = append(inbox, rec)
		}
	}
	var quarantine []map[string]string
	for _, name := range n.Home.ListDir("inbox", "quarantine") {
		if strings.HasSuffix(name, ".reason.txt") {
			continue
		}
		reason, _ := os.ReadFile(n.Home.File("inbox", "quarantine", name+".reason.txt"))
		quarantine = append(quarantine, map[string]string{
			"file": name, "reason": strings.TrimSpace(string(reason)),
		})
	}
	var drafts []respond.Draft
	for _, name := range n.Home.ListDir("outbox", "drafts") {
		var d respond.Draft
		if store.ReadJSON(n.Home.File("outbox", "drafts", name), &d) == nil {
			drafts = append(drafts, d)
		}
	}
	respondable := map[string][]string{}
	for _, rec := range inbox {
		if norm, err := archivedResponses(n, rec.MessageNumber); err == nil {
			respondable[rec.MessageNumber] = norm
		}
	}
	log := n.Home.ReadLog()
	if len(log) > 60 {
		log = log[len(log)-60:]
	}

	// The technical corner's data: partners with their addresses, the
	// rulebooks with their actual rules, the house rules, the audit diary.
	var partners []map[string]any
	for _, p := range n.Trust.ListPartners() {
		entry := map[string]any{
			"name": p.Name, "company_id": p.CompanyID,
			"key_fingerprint": p.KeyFingerprint, "first_seen": p.FirstSeen,
			"approved_via": p.ApprovedVia,
		}
		if channels, ok := n.Trust.Connections(p.CompanyID); ok {
			entry["channels"] = channels
		}
		partners = append(partners, entry)
	}
	var rulebooks []map[string]any
	for _, rt := range n.Trust.ListRulebooks() {
		entry := map[string]any{"rulebook_id": rt.RulebookID, "accepting": rt.Accepting, "retired": rt.Retired}
		if highest, ok := rt.HighestTrusted(); ok {
			if raw, err := n.Trust.TrustedRulebookBytes(rt.RulebookID, highest.Fingerprint); err == nil {
				if doc, err := supermessage.Parse(raw); err == nil {
					norm := supermessage.Normalize(doc.M).(map[string]any)
					entry["rules"] = norm["rules"]
					entry["fields"] = norm["fields"]
					entry["published_by"] = norm["published_by"]
					entry["valid_from"] = norm["valid_from"]
				}
			}
		}
		rulebooks = append(rulebooks, entry)
	}
	audit := n.Audit.Read()
	if len(audit) > 50 {
		audit = audit[len(audit)-50:]
	}
	var policy any
	store.ReadJSON(n.Home.File("policy", "review-policy.json"), &policy)

	deliveries := n.ListDeliveries()
	if len(deliveries) > 100 {
		deliveries = deliveries[:100]
	}

	return map[string]any{
		"identity":   n.Identity,
		"deliveries": deliveries,
		"alerts":     alerts(n),
		"identity_details": map[string]any{
			"key_fingerprint": sign.KeyFingerprint(n.Pub),
			"directory":       n.Identity.Directory,
			"home":            n.Home.Path,
		},
		"inbox":             inbox,
		"review":            n.Desk.Queue.ListPending(),
		"quarantine":        quarantine,
		"trusted_rulebooks": n.Trust.ListRulebooks(),
		"rulebooks":         rulebooks,
		"partners":          partners,
		"policy":            policy,
		"audit":             audit,
		"drafts":            drafts,
		"respondable":       respondable,
		"held":              n.Home.ListDir("held"),
		"log":               log,
	}
}

// alerts computes what deserves attention right now, in plain sentences.
// Severity "red" means something is wrong; "amber" means something waits.
func alerts(n *engine.Node) []map[string]string {
	now := n.Now().UTC()
	var out []map[string]string
	add := func(severity, text, tab string) {
		out = append(out, map[string]string{"severity": severity, "text": text, "tab": tab})
	}

	if c := len(n.Desk.Queue.ListPending()); c > 0 {
		add("amber", fmt.Sprintf("%d proposal(s) wait for your decision — nothing changes until you decide.", c), "decisions")
	}
	if c := len(n.Home.ListDir("held")); c > 0 {
		add("amber", fmt.Sprintf("%d message(s) are on hold behind those decisions.", c), "decisions")
	}

	// Overdue and unanswered messages.
	for _, name := range n.Home.ListDir("inbox") {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		var rec engine.InboxRecord
		if store.ReadJSON(n.Home.File("inbox", name), &rec) != nil || rec.Responded {
			continue
		}
		if rec.Status == "red" {
			add("red", fmt.Sprintf("%s breaks the rules and has no answer yet — open it and send the rejection.", rec.MessageNumber), "messages")
		}
		if rec.ResponseDeadline != "" {
			if due, err := time.Parse(time.RFC3339, rec.ResponseDeadline); err == nil && now.After(due) {
				add("red", fmt.Sprintf("The response to %s is OVERDUE (%s was promised).", rec.MessageNumber, rec.DeadlineDisplay), "messages")
			}
		}
	}

	// Deliveries in trouble, and hand-overs nobody confirmed.
	for _, d := range n.ListDeliveries() {
		switch d.State {
		case engine.DeliveryDeadLetter:
			add("red", fmt.Sprintf("Delivery of %s to %s gave up after repeated failures — retry or investigate. Last error: %s", d.MessageNumber, d.ToName, d.LastError), "deliveries")
		case engine.DeliveryFailed:
			add("amber", fmt.Sprintf("Delivery of %s to %s is failing (%s) — retrying automatically.", d.MessageNumber, d.ToName, d.LastError), "deliveries")
		case engine.DeliveryHandedOver:
			if t, err := time.Parse(time.RFC3339, d.HandedOverAt); err == nil && now.Sub(t) > 24*time.Hour {
				add("amber", fmt.Sprintf("%s was handed over to %s more than a day ago and no receipt came back — check with the partner.", d.MessageNumber, d.ToName), "deliveries")
			}
		}
	}

	if c := len(n.Home.ListDir("inbox", "quarantine")) / 2; c > 0 {
		add("red", fmt.Sprintf("%d file(s) in quarantine failed the identity check — worth a look.", c), "tech")
	}
	return out
}

// archivedResponses lists the response types the original message allows.
func archivedResponses(n *engine.Node, messageNumber string) ([]string, error) {
	raw, err := n.Home.ReadArchived(messageNumber)
	if err != nil {
		return nil, err
	}
	var m struct {
		HowToRespond struct {
			Allowed []struct {
				ResponseType string `json:"response_type"`
			} `json:"allowed_responses"`
		} `json:"how_to_respond"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	var out []string
	for _, r := range m.HowToRespond.Allowed {
		out = append(out, r.ResponseType)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no responses declared")
	}
	return out, nil
}

// Handler builds the HTTP mux for one node console.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		b, _ := consoleHTML.ReadFile("console.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})

	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		s.Mu.Lock()
		state := State(s.Node)
		s.Mu.Unlock()
		writeJSON(w, state)
	})

	mux.HandleFunc("POST /receive", func(w http.ResponseWriter, r *http.Request) {
		s.Mu.Lock()
		outcomes := s.Node.ReceiveAll()
		s.Mu.Unlock()
		writeJSON(w, map[string]any{"outcomes": outcomes})
	})

	mux.HandleFunc("POST /decision", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProposalID         string `json:"proposal_id"`
			Decision           string `json:"decision"`
			Reason             string `json:"reason"`
			As                 string `json:"as"`
			OutOfBandConfirmed bool   `json:"out_of_band_confirmed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		if req.As == "" {
			req.As = "operator"
		}
		s.Mu.Lock()
		defer s.Mu.Unlock()
		var msg string
		var err error
		if req.Decision == "approve" {
			msg, err = s.Node.Approve(req.ProposalID, req.As, req.Reason, req.OutOfBandConfirmed)
		} else {
			msg, err = s.Node.Reject(req.ProposalID, req.As, req.Reason)
		}
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"result": msg})
	})

	mux.HandleFunc("POST /respond/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageNumber string `json:"message_number"`
			ResponseType  string `json:"response_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		defer s.Mu.Unlock()
		d, err := s.Node.StartResponse(req.MessageNumber, req.ResponseType)
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, d)
	})

	mux.HandleFunc("POST /respond/finish", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DraftID string         `json:"draft_id"`
			Fills   map[string]any `json:"fills"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		defer s.Mu.Unlock()
		if len(req.Fills) > 0 {
			if err := s.Node.FillDraft(req.DraftID, req.Fills); err != nil {
				httpError(w, err)
				return
			}
		}
		msgNo, err := s.Node.FinishResponse(req.DraftID)
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"sent": msgNo})
	})

	mux.HandleFunc("POST /delivery/retry", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageNumber string `json:"message_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		d, err := s.Node.RetryDeliveryNow(req.MessageNumber)
		s.Mu.Unlock()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"result": "attempted now — state: " + d.State, "delivery": d})
	})

	mux.HandleFunc("POST /delivery/resend", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageNumber string `json:"message_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		d, err := s.Node.ResendDelivery(req.MessageNumber, "operator")
		s.Mu.Unlock()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"result": "resent (the partner ignores duplicates, so this is safe) — state: " + d.State})
	})

	mux.HandleFunc("POST /delivery/retry-failed", func(w http.ResponseWriter, r *http.Request) {
		s.Mu.Lock()
		results := s.Node.RetryAllFailed("operator")
		s.Mu.Unlock()
		writeJSON(w, map[string]any{"results": results, "count": len(results)})
	})

	mux.HandleFunc("POST /message/reprocess", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageNumber string `json:"message_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		outcome, err := s.Node.ReprocessInbound(req.MessageNumber)
		s.Mu.Unlock()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"result": "re-checked the archived bytes: " + outcome})
	})

	mux.HandleFunc("GET /journey", func(w http.ResponseWriter, r *http.Request) {
		number := r.URL.Query().Get("number")
		s.Mu.Lock()
		var events []store.LogEvent
		for _, ev := range s.Node.Home.ReadLog() {
			if ev.MessageNumber == number {
				events = append(events, ev)
			}
		}
		deliveries := s.Node.ListDeliveries()
		s.Mu.Unlock()
		var delivery any
		for _, d := range deliveries {
			if d.MessageNumber == number {
				delivery = d
			}
		}
		writeJSON(w, map[string]any{"message_number": number, "events": events, "delivery": delivery})
	})

	mux.HandleFunc("POST /audit/verify", func(w http.ResponseWriter, r *http.Request) {
		s.Mu.Lock()
		err := s.Node.Audit.Verify()
		s.Mu.Unlock()
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"result": "The diary verifies: no line has been altered since it was written."})
	})

	mux.HandleFunc("POST /reject-message", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MessageNumber string `json:"message_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, err)
			return
		}
		s.Mu.Lock()
		defer s.Mu.Unlock()
		msgNo, err := s.Node.SendRejection(req.MessageNumber)
		if err != nil {
			httpError(w, err)
			return
		}
		writeJSON(w, map[string]any{"sent": msgNo})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
