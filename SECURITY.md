# Security Policy

## Reporting a Vulnerability

Please **do not** report security vulnerabilities through GitHub Issues

Instead contact through the provided contact info at `github.com/PeacexF`:
* Telegram: `https://t.me/peaceful_origin`
* Email: `peace_work@tuta.io`

### Include:

- steps to reproduce
- proof of concept

We will respond within 30 minutes

## Scope

The full threat model is in [docs/security.md](docs/security.md). Please read it
before reporting: several properties below are documented limitations rather
than vulnerabilities.

**In scope**

- Secret material appearing in output, logs, or error messages
- Plaintext written to disk where it should not be, or left behind
- Incorrect file permissions on identities or decrypted files
- A file decrypting for a key that is not one of its recipients
- Parsing bugs that drop, alter, or misattribute a variable
- Anything that makes envseal encrypt for the wrong recipients

**Not vulnerabilities**

- Malware or any process running as the user reading `~/.envseal/identity`
- Reading a child process's environment from `/proc/<pid>/environ`
- A recipient retaining access to a file encrypted before they were removed —
  revocation is not retroactive, and the secret itself must be changed
- CI systems holding an identity that can decrypt

Vulnerabilities in [age](https://github.com/FiloSottile/age) itself should go to
that project; envseal delegates all cryptography to it.