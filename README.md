# envseal

**Commit your `.env.enc`. Keep the key on your machine. Run your app normally.**

Envseal encrypts `.env` files so they can live in Git, using
[age](https://age-encryption.org). No account, no server, no dashboard, no
network access — a single binary and a private key in your home directory.

```bash
envseal keys generate           # once per machine
envseal init                    # set up this project
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

**Homebrew**

```bash
brew install PeacexF/tap/envseal
```

**Binary** — download an archive from [Releases](https://github.com/PeacexF/envseal/releases),
verify it, and put `envseal` on your `PATH`:

```bash
sha256sum -c checksums.txt --ignore-missing
```

**From source**

```bash
go install github.com/PeacexF/envseal/cmd/envseal@latest
```

Binaries are statically linked with no runtime dependencies, for linux, macOS,
and Windows on amd64 and arm64.

## Quickstart

```bash
$ envseal keys generate
Identity created.

Public identity:
  age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p

$ envseal init
Created .envseal.yaml with your key as the only recipient
Created .env.example from .env (3 variables, no values)
Created .gitignore (+4 rules)

$ envseal encrypt .env
Encrypted .env → .env.enc
Recipients: you

$ envseal run -- ./server
```

`init` writes the `.gitignore` rules for you. The last one matters: without
`!*.enc`, the `.env.*` rule silently ignores `.env.enc` itself.

```gitignore
.env
.env.*
!.env.example
!*.enc
```

## Working with a team

Envseal synchronizes through Git itself. There is no server in the middle.

```bash
envseal push        # encrypt → commit → push
envseal pull        # fetch → decrypt → summarize what changed
```

`push` shows what it will do and asks before committing. It refuses outright if
your plaintext `.env` is tracked by Git, and it commits **only** the files it
manages, so unrelated staged work is never swept into a secrets commit.

`pull` reports what a teammate changed, by name and never by value:

```
Decrypted .env.enc → .env

+ STRIPE_WEBHOOK_SECRET
~ DATABASE_URL
```

To add someone: Alice runs `envseal keys generate`, then `envseal keys public`
and sends you the key it prints.

```bash
envseal add alice age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
envseal push
```

`add` records the key; `push` notices the pending recipient change and
re-encrypts for the new list. Alice pulls and is productive immediately:

```bash
envseal pull
envseal run -- ./server
```

## Changing one secret

```bash
$ envseal rotate STRIPE_SECRET_KEY
New value for STRIPE_SECRET_KEY:
Read 32 characters.
Rotated STRIPE_SECRET_KEY in .env.enc for 3 recipients.
```

The value is typed without echo, decrypted and re-encrypted **in memory**, and
every other line of the file survives byte for byte — comments, ordering, and
the quoting of other variables. `--generate` invents a strong value; `--stdin`
pipes one in.

There is deliberately no `--value` flag: a secret on the command line lands in
your shell history and is visible to anyone who can run `ps`.

## Reviewing and validating

`diff` makes an encrypted change reviewable without making it readable:

```bash
$ envseal diff
+ STRIPE_WEBHOOK_SECRET
~ DATABASE_URL
- OLD_API_KEY

3 changes
```

`check` validates the project and catches the mistake that matters most — a
plaintext `.env` committed to the repository:

```bash
$ envseal check
ok   configuration   2 recipients
ok   encrypted file  7 variables
ok   schema          every variable in .env.example is present
FAIL plaintext       committed to the repository, so the values are exposed
    .env.production

1 problem found.
```

Both exit non-zero on findings (`diff` needs `--exit-code`), so CI can gate on
them, and both take `--json`.

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
| `envseal keys generate` | Create your identity in `~/.envseal/identity` |
| `envseal keys public` | Print your public key, for sharing |
| `envseal init` | Set up this project: config, example, ignore rules |
| `envseal push` | Encrypt, commit, and push the environment |
| `envseal pull` | Fetch and decrypt the shared environment |
| `envseal encrypt [file]` | Encrypt `.env` for the project's recipients |
| `envseal decrypt [file]` | Decrypt back to plaintext, `-o -` for stdout |
| `envseal run [file] -- cmd` | Run a command with the decrypted environment |
| `envseal add <name> <key>` | Authorize a public key |
| `envseal remove <name>` | Withdraw a recipient |
| `envseal rotate <VAR>` | Replace one variable's value, in memory |
| `envseal reseal` | Re-encrypt for the current recipient list |
| `envseal diff` | Show which variables changed, by name only |
| `envseal check` | Validate the project, detect exposed plaintext |
| `envseal status` | Show project state, `--json` for scripts |
| `envseal completion <shell>` | bash, zsh, fish, powershell |

Global flags: `--identity <path>`, `--quiet`.

Exit codes: `0` success, `1` general, `2` configuration, `3` encryption,
`4` identity, `5` process, `6` git. `envseal run` returns the child's own exit
code.

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
