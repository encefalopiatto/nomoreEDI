// Package transport moves signed supermessage files between nodes. Three
// ways of travelling are built in: a local folder (demos and tests), HTTPS
// push (the native node-to-node way), and SFTP (what many retailers run).
// Transport only moves bytes — trust never comes from the road a file took,
// always from the signature inside it.
package transport

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Channel is one way to reach a partner, as stored in the connections file:
// {"channel":"local-folder","address":...} or
// {"channel":"https","address":"https://host:port/exchange/inbound"} or
// {"channel":"sftp","address":"host:22","folder":"/inbound","username":...,
//
//	"password":..., "host_key_fingerprint": "SHA256:..." (optional pin)}.
type Channel map[string]any

func (c Channel) Kind() string { s, _ := c["channel"].(string); return s }

func (c Channel) str(key string) string { s, _ := c[key].(string); return s }

// Deliver sends one file over one channel. An error means this attempt
// failed and may be retried; success means the bytes were handed over —
// "delivered" still waits for the partner's signed receipt.
func Deliver(c Channel, fileName string, b []byte) error {
	switch c.Kind() {
	case "local-folder":
		return sendFolder(c.str("address"), fileName, b)
	case "https":
		return sendHTTPS(c.str("address"), fileName, b)
	case "sftp":
		return sendSFTP(c, fileName, b)
	default:
		return fmt.Errorf("the prototype does not speak %q yet (it speaks local-folder, https, sftp)", c.Kind())
	}
}

// PickChannel chooses the preferred usable channel from what is on file.
func PickChannel(channels []any) (Channel, error) {
	byKind := map[string]Channel{}
	for _, raw := range channels {
		if m, ok := raw.(map[string]any); ok {
			byKind[Channel(m).Kind()] = Channel(m)
		}
	}
	for _, kind := range []string{"https", "sftp", "local-folder"} {
		if c, ok := byKind[kind]; ok {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no usable channel on file (need https, sftp, or local-folder)")
}

// ---- local folder ----

func sendFolder(address, fileName string, b []byte) error {
	if address == "" {
		return fmt.Errorf("the folder channel has no address")
	}
	if err := os.MkdirAll(address, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(address, "."+fileName+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(address, fileName))
}

// Collect drains a node's own transport/in folder, returning each file's
// bytes and removing it. Hidden temp files are skipped.
func Collect(inDir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(inDir)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(inDir, e.Name())
		b, err := os.ReadFile(full)
		if err != nil {
			return out, fmt.Errorf("could not read incoming file %s: %w", e.Name(), err)
		}
		out[e.Name()] = b
		os.Remove(full)
	}
	return out, nil
}

// ---- HTTPS push ----

var httpClient = &http.Client{Timeout: 30 * time.Second}

func sendHTTPS(address, fileName string, b []byte) error {
	if address == "" {
		return fmt.Errorf("the https channel has no address")
	}
	req, err := http.NewRequest("POST", address, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Supermessage-Filename", fileName)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("the partner's node answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// InboundHTTPHandler accepts supermessages pushed by partners and hands each
// one to the node's pipeline. Plain bytes in, one status line out — trust
// comes from the signature check inside the pipeline, never from the
// connection.
func InboundHTTPHandler(ingest func(fileName string, b []byte) string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /exchange/inbound", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 32<<20)) // 32 MB cap
		if err != nil || len(b) == 0 {
			http.Error(w, "empty or unreadable body", http.StatusBadRequest)
			return
		}
		name := r.Header.Get("X-Supermessage-Filename")
		if name == "" || strings.ContainsAny(name, "/\\") {
			name = fmt.Sprintf("https-%d.supermessage.json", time.Now().UnixNano())
		}
		outcome := ingest(name, b)
		w.Write([]byte(outcome + "\n"))
	})
	return mux
}

// StartInboundHTTP runs the inbound listener until it fails.
func StartInboundHTTP(addr, certFile, keyFile string, ingest func(fileName string, b []byte) string) error {
	srv := &http.Server{Addr: addr, Handler: InboundHTTPHandler(ingest), ReadTimeout: 60 * time.Second}
	if certFile != "" && keyFile != "" {
		return srv.ListenAndServeTLS(certFile, keyFile)
	}
	return srv.ListenAndServe()
}

// ---- SFTP client ----

func sendSFTP(c Channel, fileName string, b []byte) error {
	address := c.str("address")
	if address == "" {
		return fmt.Errorf("the sftp channel has no address")
	}
	if !strings.Contains(address, ":") {
		address += ":22"
	}
	user := c.str("username")
	pass := c.str("password")
	if user == "" || pass == "" {
		return fmt.Errorf("the sftp channel needs username and password on file (a technician sets these locally; they never travel inside messages)")
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: hostKeyCheck(c.str("host_key_fingerprint")),
		Timeout:         20 * time.Second,
	}
	conn, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return fmt.Errorf("could not reach the partner's sftp server: %w", err)
	}
	defer conn.Close()
	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer client.Close()
	folder := c.str("folder")
	if folder == "" {
		folder = "."
	}
	tmp := path(folder, "."+fileName+".tmp")
	f, err := client.Create(tmp)
	if err != nil {
		return fmt.Errorf("could not write into the partner's folder %q: %w", folder, err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	final := path(folder, fileName)
	client.Remove(final) // a leftover from an earlier attempt must not block the rename
	return client.Rename(tmp, final)
}

// hostKeyCheck pins the partner server's host key when a fingerprint is on
// file; without one, the first connection accepts and the mismatch case is
// what the fingerprint field exists to close.
func hostKeyCheck(pinned string) ssh.HostKeyCallback {
	if pinned == "" {
		return ssh.InsecureIgnoreHostKey() // prototype default, called out in NODE.md
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		if got != pinned {
			return fmt.Errorf("the partner server's identity changed (host key %s, expected %s) — refusing", got, pinned)
		}
		return nil
	}
}

func path(dir, name string) string {
	return strings.TrimRight(dir, "/") + "/" + name
}
