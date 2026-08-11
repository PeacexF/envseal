// Package crypto wraps age encryption. It implements no cryptography of its
// own: every operation is delegated to filippo.io/age.
package crypto

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/PeacexF/envseal/internal/errs"
)

// Encrypt returns ASCII-armored age ciphertext, which stays diffable in Git.
func Encrypt(plaintext []byte, recipients []age.Recipient) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errs.New(errs.CodeCrypto, "no recipients to encrypt for")
	}

	fail := func(err error) error {
		return errs.New(errs.CodeCrypto, "unable to encrypt").Wrap(err)
	}

	var out bytes.Buffer
	armored := armor.NewWriter(&out)

	w, err := age.Encrypt(armored, recipients...)
	if err != nil {
		return nil, fail(err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fail(err)
	}
	if err := w.Close(); err != nil {
		return nil, fail(err)
	}
	if err := armored.Close(); err != nil {
		return nil, fail(err)
	}
	return out.Bytes(), nil
}

// Decrypt reads armored or binary age ciphertext. source names the file for
// error messages; neither the ciphertext nor the plaintext appears in errors.
func Decrypt(ciphertext []byte, identities []age.Identity, source string) ([]byte, error) {
	if len(identities) == 0 {
		return nil, errs.New(errs.CodeIdentity, "no identity available to decrypt %s", source)
	}

	var src io.Reader = bytes.NewReader(ciphertext)
	if bytes.HasPrefix(bytes.TrimLeft(ciphertext, " \t\r\n"), []byte(armor.Header)) {
		src = armor.NewReader(src)
	}

	r, err := age.Decrypt(src, identities...)
	if err != nil {
		return nil, decryptError(err, source)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, decryptError(err, source)
	}
	return plaintext, nil
}

// IsEncrypted reports whether data looks like an age file.
func IsEncrypted(data []byte) bool {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte(armor.Header)) ||
		bytes.HasPrefix(trimmed, []byte("age-encryption.org/"))
}

func decryptError(err error, source string) error {
	var noMatch *age.NoIdentityMatchError
	if errors.As(err, &noMatch) {
		return errs.New(errs.CodeCrypto, "unable to decrypt %s", source).
			Detailf("The file was not encrypted for the identity you are using.").
			Check("confirm your public key is listed in .envseal.yaml",
				"ask a current recipient to run `envseal add <name> <your key>` and `envseal rotate`",
				"check --identity and ENVSEAL_IDENTITY point at the right identity")
	}

	var armorErr *armor.Error
	if errors.As(err, &armorErr) || strings.Contains(err.Error(), "parsing age header") {
		return errs.New(errs.CodeCrypto, "unable to decrypt %s", source).
			Detailf("The file is not valid age ciphertext, or it was damaged in transit.").
			Check("confirm the file was not modified by a merge or an editor",
				"restore it from Git history")
	}

	return errs.New(errs.CodeCrypto, "unable to decrypt %s", source).Wrap(err)
}
