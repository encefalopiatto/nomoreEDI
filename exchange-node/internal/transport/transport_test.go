package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// A supermessage pushed over HTTPS must reach the ingest function byte-exact.
func TestHTTPSRoundtrip(t *testing.T) {
	var gotName string
	var gotBytes []byte
	ts := httptest.NewServer(InboundHTTPHandler(func(name string, b []byte) string {
		gotName, gotBytes = name, b
		return "delivered green"
	}))
	defer ts.Close()

	payload := []byte(`{"hello":"world"}`)
	ch := Channel{"channel": "https", "address": ts.URL + "/exchange/inbound"}
	if err := Deliver(ch, "MSG-1.supermessage.json", payload); err != nil {
		t.Fatalf("https delivery failed: %v", err)
	}
	if gotName != "MSG-1.supermessage.json" || string(gotBytes) != string(payload) {
		t.Fatalf("the server received %q / %q", gotName, gotBytes)
	}

	// A refusing server must surface as a failed attempt.
	bad := Channel{"channel": "https", "address": ts.URL + "/nowhere"}
	if err := Deliver(bad, "MSG-2.supermessage.json", payload); err == nil {
		t.Fatal("a 404 from the partner must fail the attempt")
	}
}

// A file uploaded through the SFTP drop folder must land in the in-directory,
// and a wrong password must be turned away.
func TestSFTPRoundtrip(t *testing.T) {
	inDir := t.TempDir()
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go ServeSFTP(listener, SFTPServerConfig(map[string]string{"weide": "grasgruen"}, signer), inDir)

	ch := Channel{
		"channel": "sftp", "address": listener.Addr().String(),
		"username": "weide", "password": "grasgruen", "folder": "/",
	}
	payload := []byte(`{"an":"upload"}`)
	if err := Deliver(ch, "MSG-3.supermessage.json", payload); err != nil {
		t.Fatalf("sftp delivery failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(inDir, "MSG-3.supermessage.json")); err == nil {
			got = b
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if string(got) != string(payload) {
		t.Fatalf("the uploaded file did not arrive intact, got %q", got)
	}

	wrong := Channel{
		"channel": "sftp", "address": listener.Addr().String(),
		"username": "weide", "password": "falsch", "folder": "/",
	}
	if err := Deliver(wrong, "MSG-4.supermessage.json", payload); err == nil {
		t.Fatal("a wrong password must be turned away")
	}
}
