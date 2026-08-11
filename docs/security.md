# Security

What envseal protects, what it does not, and how it behaves internally. Read the
threat model before you decide this tool fits your situation.

## What envseal actually does

It encrypts a file for a list of public keys, using
[age](https://age-encryption.org), and hands the decrypted contents to a child
process. That is all. Envseal implements no cryptography of its own — no custom
constructions, no hand-rolled primitives, no key derivation of its own design.

## Threat model

### Protects against

- **Accidental commits.** The plaintext never needs to exist in the repository.
- **Repository exposure.** A leaked clone, a mistakenly public repo, or a
  compromised Git host yields ciphertext. Without a recipient's private key it
  is not readable.
- **Unauthorized readers with repo access.** A contractor with commit rights but
  no recipient key cannot read the environment.
- **Insecure distribution.** `.env` files stop travelling through Slack and
  email.
- **Casual local exposure.** Files at rest in a project directory are encrypted.

### Does **not** protect against

Stated plainly, because a security tool that oversells itself is worse than none:

- **Malware running as you.** Anything that can read `~/.envseal/identity` has
  your access. Envseal is not a sandbox.
- **A compromised operating system.** Same reasoning.
- **A stolen unlocked device.** The identity is a file on disk. Full-disk
  encryption and screen locking are your defence, not envseal.
- **A leaked private identity.** Whoever holds it can decrypt everything it is a
  recipient of, forever, including old commits.
- **Other processes reading the child's environment.** On Linux, a process
  running as the same user can read `/proc/<pid>/environ`. `envseal run` avoids
  writing plaintext to *disk*; it cannot hide it from the operating system.
- **Compromised CI.** Giving a build system an identity gives it the secrets.
  Anyone who can run a workflow can potentially exfiltrate them.
- **Revocation of what was already shared.** See below — this one matters most.
- **Someone deliberately sharing their key.** No technical control prevents it.
- **Weak secrets.** Encrypting a password of `admin` protects it in transit and
  at rest, and not at all from being guessed.

### Revocation is not retroactive

This is the most commonly misunderstood property of every tool in this category:

> `envseal remove` plus `envseal reseal` controls **future** encryptions. It
> does not retract a copy someone already has. Their key still opens the old
> `.env.enc`, and Git history keeps that file forever.
>
> To genuinely revoke access to a secret, **change the secret at its source** —
> issue a new API key, reset the password, roll the token — and then encrypt the
> new value.

Envseal says this in the output of `envseal remove` for the same reason it is
here: skipping it is the single most likely way to end up feeling secure while
not being secure.

## How envseal behaves

### Secrets never appear in output

- Values are never printed, logged, or included in an error message. Errors
  refer to line numbers, variable names, and file paths.
- A malformed `.env` line is reported as `Line 4: expected KEY=value` — never
  quoted back, because the line may itself be a secret.
- A malformed identity is never echoed, for the same reason.
- Formatting an identity value in Go yields the **public** key, so even a stray
  `%v` cannot leak the private one.
- `envseal status`, including `--json`, reports only paths, recipient names, and
  public keys.

Each of these is enforced by a test that fails if a known secret appears in any
command's output.

### Printing to a terminal is refused

`envseal decrypt -o -` refuses to write to a terminal, because secrets would
remain in scrollback and terminal buffers after the command finished. Redirect
or pipe the output, or pass `--force` to override deliberately.

### File permissions

| File | Mode |
|---|---|
| `~/.envseal/` | `0700` |
| `~/.envseal/identity` | `0600` |
| Decrypted plaintext | `0600` |
| `.env.enc`, `.envseal.yaml` | `0644` (meant to be committed) |
| `~/.envseal/sync/*` | `0600` |

Envseal warns when an identity file is group- or world-readable. It warns rather
than refusing, because CI checkouts sometimes have modes you cannot control.
Windows uses its own access control and these checks are skipped there.

### Temporary files

Every write is atomic: content is written to a temporary file in the
**destination directory** and renamed into place, so a crash never leaves a
half-written `.env.enc` or a truncated identity.

This means decrypted content does briefly exist under a second name. The
mitigations:

- created with mode `0600` from the outset, never widened before the content is
  written;
- placed in the destination directory, never in a world-writable location such
  as `/tmp`;
- removed on every error path.

Only an uncatchable kill (`SIGKILL`, power loss) between write and rename can
leave residue, and that residue is owner-only. The alternative — non-atomic
writes — trades this narrow window for corrupted files on any interruption,
which is worse.

### Editing a sealed value

`envseal rotate` decrypts in memory, replaces one value, and encrypts again;
plaintext never reaches the disk. Two properties make that safe to rely on:

- The value is never accepted as a command-line argument. There is no `--value`
  flag, because arguments are written to shell history and are visible in `ps`
  to every user on the machine. It is typed without echo, piped via `--stdin`,
  or generated.
- A value containing a NUL byte is refused. The operating system marks the end
  of an environment value with one, so writing it would produce a sealed file
  that nothing could read back.

If a local `.env` exists and still matches the sealed environment, it is updated
too. That writes plaintext, but the alternative is worse: a stale `.env` would be
re-encrypted by the next `push`, silently undoing the rotation.

### Sync fingerprints

`envseal pull` records a SHA-256 of each plaintext file it writes, under
`~/.envseal/sync/`, so a later pull can tell an untouched file from one you
edited. It stores the hash, never the content, and never in the repository. A
hash confirms a guess but does not reveal a secret, and the directory sits
alongside your private key with the same permissions.

### Memory

`envseal run` zeroes the decrypted buffer once the environment has been built.
This narrows the window; it does not close it. Go strings are immutable and
copies persist until garbage collected, and the environment must exist in memory
for the child to receive it. Envseal does not claim to defend against an
attacker who can read its process memory — such an attacker has already won.

### Process handling

`envseal run` places the child in its own process group and forwards `SIGINT`,
`SIGTERM`, `SIGHUP`, and `SIGQUIT`, so the whole tree it spawns is signalled.
The child's exit code becomes envseal's; a child killed by a signal reports
`128 + signal number`.

### Dependencies

The complete list:

| Dependency | Why |
|---|---|
| `filippo.io/age` | All encryption, decryption, and key handling |
| `gopkg.in/yaml.v3` | Reading `.envseal.yaml` |
| `github.com/spf13/cobra` | Command-line parsing and completion |
| `golang.org/x/term` | Detecting whether output is a terminal |

`envseal push`, `pull`, `diff --ref`, and `check` additionally invoke the
**`git` binary** you already have. Shelling out rather than embedding a Git
library means your credentials, SSH agent, signing keys, and hooks keep working,
and envseal never handles them.

Two consequences worth stating:

- Envseal runs the `git` it finds on your `PATH`. A malicious binary earlier in
  `PATH` would be run, exactly as it would be if you typed `git` yourself.
- `git commit` and `git push` run the repository's own hooks. Cloning a
  repository and running any git command already does this; envseal does not
  change it, and does not disable hooks.

Arguments are passed as separate values, never through a shell, so there is no
shell injection. Paths are additionally guarded with a `--` separator so a
filename can never be read as an option. A **revision** cannot be guarded that
way — `git show <rev>:<path>` joins the two into one argument — so revisions
beginning with a dash are rejected outright.

A small dependency surface is a security property. CI runs `govulncheck` on
every push.

### Testing

Security-relevant behaviour is covered by tests rather than by intent:

- no secret or `AGE-SECRET-KEY-` appears in any command's stdout or stderr;
- file modes are asserted after every write;
- `envseal run` leaves no plaintext in the project directory or in `TMPDIR`;
- `envseal push` refuses when the plaintext `.env` is tracked by git, commits
  only the files it manages, and its pushed output is verified to be armored
  ciphertext containing no secret;
- corrupted, truncated, and foreign-recipient files fail with exit 3;
- the `.env` parser is fuzzed — tens of millions of executions, checking that
  names stay usable and that the source bytes are never altered;
- the `.env` **editor** is fuzzed the way `rotate` drives it: whatever the file
  looks like, replacing one value stores exactly that value, leaves every other
  variable untouched, and produces a file that still parses.

## Verifying envseal's work yourself

Envseal writes standard age files. You do not have to take its word for
anything:

```bash
head -1 .env.enc          # -----BEGIN AGE ENCRYPTED FILE-----
age -d -i ~/.envseal/identity .env.enc
```

If envseal vanished, `age` alone would still open every file it produced.

## Reporting a vulnerability

See [SECURITY.md](../SECURITY.md) in the repository root.
