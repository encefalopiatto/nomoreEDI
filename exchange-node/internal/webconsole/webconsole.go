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

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/engine"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/respond"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
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
	if len(log) > 40 {
		log = log[len(log)-40:]
	}
	return map[string]any{
		"identity":          n.Identity,
		"inbox":             inbox,
		"review":            n.Desk.Queue.ListPending(),
		"quarantine":        quarantine,
		"trusted_rulebooks": n.Trust.ListRulebooks(),
		"drafts":            drafts,
		"respondable":       respondable,
		"held":              n.Home.ListDir("held"),
		"log":               log,
	}
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
