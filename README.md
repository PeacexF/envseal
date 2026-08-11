# envseal

**Commit your `.env.enc`. Keep the key on your machine. Run your app normally.**

Envseal encrypts `.env` files so they can live in Git, using
[age](https://age-encryption.org). No account, no server, no dashboard, no
network access — a single binary and a private key in your home directory.

```bash
envseal init                    # once per machine
envseal encrypt .env            # → .env.enc, safe to commit
envseal run -- ./server         # decrypted in memory, never on disk
```

---

## Why

`.env` files are the last thing in a project that cannot be committed. So they
travel by Slack, get pasted into tickets, drift between machines, and go stale
in a password manager nobody updates.

The alternative is usually a hosted secrets manager: an account, a subscription,
a network dependency, and an outage that stops your team from starting a local
server.

Envseal takes the third option. Your secrets live in the repository, encrypted
for a list of public keys. Everyone who is authorized just runs the app.

## How it works

```
      Repository                     Your machine
   ┌──────────────┐              ┌──────────────────┐
   │  .env.enc    │              │ ~/.envseal/      │
   │ .envseal.yaml│              │   identity       │  ← private, never shared
   └──────┬───────┘              └────────┬─────────┘
          │                               │
          └──────────► envseal ◄──────────┘
                          │
                 decrypted in memory
                          │
                          ▼
                    your process
```

`.envseal.yaml` lists the public keys allowed to decrypt. `.env.enc` is
ASCII-armored age ciphertext, so Git treats it as an ordinary text file.

> **Handing someone `.env.enc` gives them nothing.** It only opens for a private
> identity whose public key was a recipient when the file was encrypted.

## Install

```bash
go install github.com/PeacexF/envseal/cmd/envseal@latest
```

Or from a clone:

```bash
make build      # → bin/envseal
make install    # → $GOPATH/bin
```

Binary releases and a Homebrew formula are planned.

## Quickstart

```bash
$ envseal init
Identity created.

Public identity:
  age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p

$ envseal encrypt .env
Created .envseal.yaml with your identity as the only recipient.

Encrypted .env → .env.enc
Recipients: you

Commit .env.enc and .envseal.yaml. Keep .env out of Git.

$ envseal run -- ./server
```

Add to `.gitignore`:

```gitignore
.env
.env.*
!.env.example
!*.enc
```

## Working with a team

Alice runs `envseal init` and sends you her **public** key. You authorize it:

```bash
envseal add alice age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
envseal rotate
git commit -am "Grant alice access"
```

`add` records the key; `rotate` re-encrypts for the current list. They are
separate steps on purpose — editing a config file and re-encrypting every secret
in the repository should not be one keystroke.

Alice pulls and is productive immediately:

```bash
envseal run -- ./server
```

## CI

```yaml
- name: Run tests
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal run -- go test ./...
```

`ENVSEAL_IDENTITY` accepts a path **or** the key material itself, so CI needs no
temporary files. Use a dedicated CI identity, never a personal one. See
[docs/ci.md](docs/ci.md).

## Commands

| Command | Purpose |
|---|---|
| `envseal init` | Create your identity in `~/.envseal/identity` |
| `envseal encrypt [file]` | Encrypt `.env` for the project's recipients |
| `envseal decrypt [file]` | Decrypt back to plaintext, `-o -` for stdout |
| `envseal run [file] -- cmd` | Run a command with the decrypted environment |
| `envseal add <name> <key>` | Authorize a public key |
| `envseal remove <name>` | Withdraw a recipient |
| `envseal rotate` | Re-encrypt for the current recipient list |
| `envseal status` | Show project state, `--json` for scripts |
| `envseal completion <shell>` | bash, zsh, fish, powershell |

Global flags: `--identity <path>`, `--quiet`.

Exit codes: `0` success, `1` general, `2` configuration, `3` encryption,
`4` identity, `5` process. `envseal run` returns the child's own exit code.

## What it does not do

Envseal protects secrets against accidental commits, repository exposure, and
`.env` files travelling through chat. It does **not** protect against:

- malware or anything else running as you — it can read your identity file;
- a stolen unlocked device;
- a leaked private key, which opens everything it is a recipient of, forever;
- other processes on the machine reading the child's environment;
- compromised CI, which by definition holds a key.

And one property that surprises people:

> **Removing a recipient does not un-share what they already have.** Their copy
> of the old `.env.enc` still opens with their key, and Git history keeps it
> forever. Rotation controls *future* encryptions. To truly revoke access,
> change the secret at its source.

The full threat model is in [docs/security.md](docs/security.md). Read it before
adopting this for anything that matters.

## Documentation

- **[Guide](docs/guide.md)** — the full walkthrough, start here
- [Configuration](docs/configuration.md) — every `.envseal.yaml` field
- [Key management](docs/key-management.md) — identities, rotation, recovery
- [CI](docs/ci.md) — GitHub Actions, GitLab, Jenkins, Docker, deployment
- [Security](docs/security.md) — threat model and internal behaviour
- [Architecture](docs/architecture.md) — how the code fits together

## Design

- **Local-first.** Works entirely offline. No account, no server, no telemetry.
- **Boring cryptography.** All encryption is [age](https://age-encryption.org).
  Envseal implements none of its own.
- **No lock-in.** Standard age files. `age -d -i ~/.envseal/identity .env.enc`
  works today and would still work if this project disappeared.
- **Secrets never printed.** Not in output, not in logs, not in error messages —
  enforced by tests, not intentions.
- **Git-friendly.** ASCII armor, so encrypted files diff, merge, and paste.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Tests are black-box, run
`make check` before opening a pull request.

## License

[MIT](LICENSE)
