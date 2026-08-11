package crypto_test

import (
	"bytes"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/PeacexF/envseal/internal/crypto"
	"github.com/PeacexF/envseal/internal/errs"
)

const plaintext = "DATABASE_URL=postgres://localhost/app\nAPI_KEY=s3cret\n"

func newKey(t *testing.T) *age.X25519Identity {
	t.Helper()
	key, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func encrypt(t *testing.T, data string, keys ...*age.X25519Identity) []byte {
	t.Helper()

	recipients := make([]age.Recipient, len(keys))
	for i, k := range keys {
		recipients[i] = k.Recipient()
	}

	ciphertext, err := crypto.Encrypt([]byte(data), recipients)
	if err != nil {
		t.Fatalf("Encrypt() = %v", err)
	}
	return ciphertext
}

func rendered(err error) string {
	var b strings.Builder
	errs.Render(&b, err)
	return b.String()
}

func TestRoundTrip(t *testing.T) {
	key := newKey(t)

	got, err := crypto.Decrypt(encrypt(t, plaintext, key), []age.Identity{key}, ".env.enc")
	if err != nil {
		t.Fatalf("Decrypt() = %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestRoundTripEmpty(t *testing.T) {
	key := newKey(t)

	got, err := crypto.Decrypt(encrypt(t, "", key), []age.Identity{key}, ".env.enc")
	if err != nil {
		t.Fatalf("Decrypt() = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Decrypt() = %q, want empty", got)
	}
}

// Armored output is what keeps .env.enc diffable in Git.
func TestEncryptIsArmored(t *testing.T) {
	ciphertext := encrypt(t, plaintext, newKey(t))

	if !bytes.HasPrefix(ciphertext, []byte("-----BEGIN AGE ENCRYPTED FILE-----")) {
		t.Errorf("ciphertext starts with %q, want an armor header", firstLine(ciphertext))
	}
	for _, b := range ciphertext {
		if b > 0x7e || (b < 0x20 && b != '\n' && b != '\r') {
			t.Fatalf("ciphertext contains non-printable byte %#x, want ASCII armor", b)
			return
		}
	}
}

func TestEncryptIsNotPlaintext(t *testing.T) {
	ciphertext := encrypt(t, plaintext, newKey(t))

	if bytes.Contains(ciphertext, []byte("s3cret")) {
		t.Error("ciphertext contains the plaintext")
	}
}

func TestMultipleRecipients(t *testing.T) {
	alice, bob := newKey(t), newKey(t)
	ciphertext := encrypt(t, plaintext, alice, bob)

	for name, key := range map[string]*age.X25519Identity{"alice": alice, "bob": bob} {
		got, err := crypto.Decrypt(ciphertext, []age.Identity{key}, ".env.enc")
		if err != nil {
			t.Errorf("Decrypt() as %s = %v", name, err)
			continue
		}
		if string(got) != plaintext {
			t.Errorf("Decrypt() as %s = %q, want %q", name, got, plaintext)
		}
	}
}

func TestDecryptWrongIdentity(t *testing.T) {
	ciphertext := encrypt(t, plaintext, newKey(t))

	_, err := crypto.Decrypt(ciphertext, []age.Identity{newKey(t)}, ".env.enc")
	if err == nil {
		t.Fatal("Decrypt() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeCrypto {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeCrypto)
	}

	out := rendered(err)
	for _, want := range []string{".env.enc", "not encrypted for the identity", ".envseal.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("error =\n%s\nwant it to mention %q", out, want)
		}
	}
}

func TestDecryptCorrupted(t *testing.T) {
	ciphertext := encrypt(t, plaintext, newKey(t))
	key := newKey(t)

	tests := map[string][]byte{
		"truncated": ciphertext[:len(ciphertext)/2],
		"garbage":   []byte("not an age file at all\n"),
		"empty":     nil,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := crypto.Decrypt(data, []age.Identity{key}, ".env.enc"); err == nil {
				t.Fatal("Decrypt() = nil, want an error")
			} else if got := errs.CodeOf(err); got != errs.CodeCrypto {
				t.Errorf("CodeOf() = %d, want %d", got, errs.CodeCrypto)
			}
		})
	}
}

// Binary age files, as produced by `age -e`, must still decrypt.
func TestDecryptBinaryAgeFile(t *testing.T) {
	key := newKey(t)

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, key.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(plaintext)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := crypto.Decrypt(buf.Bytes(), []age.Identity{key}, ".env.enc")
	if err != nil {
		t.Fatalf("Decrypt() = %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecryptErrorNeverContainsSecrets(t *testing.T) {
	ciphertext := encrypt(t, plaintext, newKey(t))

	_, err := crypto.Decrypt(ciphertext, []age.Identity{newKey(t)}, ".env.enc")
	if err == nil {
		t.Fatal("Decrypt() = nil, want an error")
	}
	if out := rendered(err); strings.Contains(out, "s3cret") || strings.Contains(out, "AGE-SECRET-KEY-") {
		t.Errorf("error leaks material:\n%s", out)
	}
}

func TestEncryptWithoutRecipients(t *testing.T) {
	_, err := crypto.Encrypt([]byte(plaintext), nil)
	if err == nil {
		t.Fatal("Encrypt() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeCrypto {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeCrypto)
	}
}

func TestDecryptWithoutIdentities(t *testing.T) {
	_, err := crypto.Decrypt(encrypt(t, plaintext, newKey(t)), nil, ".env.enc")
	if err == nil {
		t.Fatal("Decrypt() = nil, want an error")
	}
	if got := errs.CodeOf(err); got != errs.CodeIdentity {
		t.Errorf("CodeOf() = %d, want %d", got, errs.CodeIdentity)
	}
}

func TestIsEncrypted(t *testing.T) {
	if !crypto.IsEncrypted(encrypt(t, plaintext, newKey(t))) {
		t.Error("IsEncrypted(armored) = false, want true")
	}
	if crypto.IsEncrypted([]byte(plaintext)) {
		t.Error("IsEncrypted(plaintext) = true, want false")
	}
}

func firstLine(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return string(data[:i])
	}
	return string(data)
}
