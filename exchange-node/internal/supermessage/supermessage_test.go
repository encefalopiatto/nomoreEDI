package supermessage

import (
	"os"
	"testing"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
)

func specDoc(t *testing.T) *Doc {
	t.Helper()
	b, err := os.ReadFile("../../../spec/example-order.supermessage.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The fingerprint must not depend on how the JSON happened to be laid out.
func TestFingerprintStableAcrossReserialization(t *testing.T) {
	doc := specDoc(t)
	rb, _ := doc.Section("rulebook")
	fp1, err := Fingerprint(rb)
	if err != nil {
		t.Fatal(err)
	}
	redoc, err := Parse(MarshalPretty(doc.M))
	if err != nil {
		t.Fatal(err)
	}
	rb2, _ := redoc.Section("rulebook")
	fp2, err := Fingerprint(rb2)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprint changed across re-serialization: %s vs %s", fp1, fp2)
	}
}

// Flipping one value after signing must break the signature.
func TestTamperDetection(t *testing.T) {
	doc := specDoc(t)
	dir := t.TempDir()
	pub, err := sign.Generate(dir+"/priv.key", dir+"/pub.key")
	if err != nil {
		t.Fatal(err)
	}
	priv, err := sign.LoadPrivate(dir + "/priv.key")
	if err != nil {
		t.Fatal(err)
	}
	signing, err := FileSigningBytes(doc.M)
	if err != nil {
		t.Fatal(err)
	}
	sig := sign.Sign(priv, signing)
	if !sign.Verify(pub, signing, sig) {
		t.Fatal("a fresh signature must verify")
	}
	// Tamper: change the ordered quantity of line 1.
	content, _ := doc.Section("content")
	lines := content["lines"].([]any)
	lines[0].(map[string]any)["ordered_quantity"] = "9999"
	tampered, err := FileSigningBytes(doc.M)
	if err != nil {
		t.Fatal(err)
	}
	if sign.Verify(pub, tampered, sig) {
		t.Fatal("a tampered file must NOT verify")
	}
}
