# Envseal Guide

A complete walkthrough: from your first encrypted file to running a team, a CI
pipeline, and a production deployment.

If you want the short version, read [Your first project](#your-first-project)
and [Daily use](#daily-use). Everything after that is there when you need it.

## Contents

1. [Install](#install)
2. [The four pieces](#the-four-pieces)
3. [Your first project](#your-first-project)
4. [What to commit](#what-to-commit)
5. [Daily use](#daily-use)
6. [Sharing through Git](#sharing-through-git)
7. [Reviewing and validating](#reviewing-and-validating)
8. [Changing a secret](#changing-a-secret)
9. [Working with other people](#working-with-other-people)
10. [Removing someone](#removing-someone)
11. [Multiple environments](#multiple-environments)
12. [Docker and Compose](#docker-and-compose)
13. [Continuous integration](#continuous-integration)
14. [Scripting envseal](#scripting-envseal)
15. [Shell completion](#shell-completion)
16. [The .env format](#the-env-format)
17. [Exit codes](#exit-codes)
18. [Troubleshooting](#troubleshooting)

---

## Install

Envseal is a single statically linked binary with no runtime dependencies.

```bash
brew install PeacexF/tap/envseal
```

Or download an archive from [Releases](https://github.com/PeacexF/envseal/releases)
and verify it against `checksums.txt`:

```bash
sha256sum -c checksums.txt --ignore-missing
```

Or install from source:

```bash
go install github.com/PeacexF/envseal/cmd/envseal@latest
```

Or build from a clone:

```bash
git clone https://github.com/PeacexF/envseal
cd envseal
make build          # produces bin/envseal
make install        # or install into $GOPATH/bin
```

Check it:

```bash
envseal --version
```

---

## The four pieces

Envseal has a small vocabulary. Learning these four words makes everything else
obvious.

| Piece | Where it lives | Secret? | What it is |
|---|---|---|---|
| **Identity** | `~/.envseal/identity` | **Yes** | Your private key. It decrypts. Never leaves your machine. |
| **Public key** | `envseal keys public` | No | Derived from your identity. Give it to anyone. |
| **Encrypted file** | `.env.enc` in your repo | No | Your secrets, encrypted for a list of public keys. Commit it. |
| **Configuration** | `.envseal.yaml` in your repo | No | Which public keys may decrypt. Commit it. |

The relationship in one line:

```
.env  --encrypt for recipients-->  .env.enc  --decrypt with your identity-->  .env
```

The single most important consequence:

> **Handing someone `.env.enc` gives them nothing.** They can only read it if
> their public key was among the recipients when the file was encrypted.

Encryption is done by [age](https://age-encryption.org), a small, modern,
well-reviewed encryption tool. Envseal implements no cryptography of its own.

---

## Your first project

Start with a normal `.env` file:

```bash
cat .env
```

```env
DATABASE_URL=postgres://localhost/app
API_KEY=s3cret
DEBUG=false
```

### 1. Create your identity

Once per machine, not once per project:

```bash
envseal keys generate
```

```
Identity created.

Private identity:
  ~/.envseal/identity

Public identity:
  age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p

Share the public identity freely. Never share or commit the private one.
```

The private identity is written with owner-only permissions (`0600`) inside a
`0700` directory. Envseal never prints it.

If an identity already exists, it refuses rather than replacing it, because
overwriting a private key permanently destroys access to everything encrypted
for it. `envseal keys generate --force` replaces it *and* keeps the old one as a
timestamped `.bak` file beside it.

### 2. Set the project up

```bash
envseal init
```

```
Created .envseal.yaml with your key as the only recipient
Created .env.example from .env (3 variables, no values)
Created .gitignore (+4 rules)

Next: `envseal encrypt .env` to seal your environment.
```

This writes the configuration, an example file listing your variable **names**
with empty values, and the ignore rules below. It is safe to run again: existing
files are reported and left alone.

### 3. Encrypt

```bash
envseal encrypt .env
```

```
Encrypted .env → .env.enc
Recipients: you
```

Two files appeared. `.env.enc` holds the ciphertext:

```
-----BEGIN AGE ENCRYPTED FILE-----
YWdlLWVuY3J5cHRpb24ub3JnL3YxCi0+IFgyNTUxOSA2NXVmL25BZGJNOHpXdDhv
bTlKaS84Q1FqNUwvcDlnWTRXRHhsNHVFM0RnClVKeHdXYXZMdnlwVFN6d05qMEZi
-----END AGE ENCRYPTED FILE-----
```

It is ASCII text on purpose. Git treats it as a text file, so it diffs, merges,
and pastes without complaint.

And `.envseal.yaml` records who may read it:

```yaml
# envseal project configuration
# Public information only. Never put secret values in this file.

version: 1
file: .env.enc
recipients:
  - name: you
    key: age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
```

### 4. Check your work

```bash
envseal status
```

```
Project
──────────────────────────────────────────────
Configuration   .envseal.yaml                 ✓
Encrypted file  .env.enc                      ✓
Identity        ~/.envseal/identity           ✓
Local access    you can decrypt this project  ✓

Recipients (1)
  you
```

That took under a minute, and there is no account, server, or dashboard
involved.

---

## What to commit

**Commit:** `.env.enc`, `.envseal.yaml`, and a `.env.example` with fake values
so newcomers know which variables exist.

**Never commit:** `.env`, any decrypted output, and `~/.envseal/identity`.

Add this to `.gitignore`:

```gitignore
.env
.env.*
!.env.example
!*.enc
```

Read that carefully, because the order matters:

- `.env` ignores the plaintext file.
- `.env.*` ignores `.env.local`, `.env.production`, and friends.
- `!.env.example` un-ignores the example file so it *is* committed.
- `!*.enc` un-ignores every encrypted file, which `.env.*` would otherwise have
  swallowed.

That last line matters more than it looks. Without it, `.env.*` matches
`.env.enc` and Git silently ignores the very file you meant to commit — you push,
your teammate pulls, and there are no secrets there at all. Note also that
`!.env.*.enc` does **not** rescue `.env.enc`, because the `*` needs something
between the dots; `!*.enc` covers both.

If your project already uses different names, adapt the pattern rather than
copying it blindly, then verify before committing:

```bash
git check-ignore -v .env        # should print the matching rule
git check-ignore -v .env.enc    # should print nothing at all
```

---

## Daily use

The best way to use a sealed environment is never to decrypt it to disk at all:

```bash
envseal run -- ./server
envseal run -- npm run dev
envseal run -- go test ./...
envseal run -- python manage.py migrate
```

Envseal decrypts in memory, hands the variables to the child process, and gets
out of the way. Your program reads `os.Getenv`, `process.env`, or `os.environ`
exactly as it would with a plain `.env` loaded by a shell.

Everything after `--` is your command. The separator is required, so there is
never any doubt about where envseal's arguments end and yours begin.

**Exit codes pass through.** `envseal run -- sh -c 'exit 7'` exits 7. This makes
it safe in scripts, `make` targets, and CI steps.

**Signals pass through.** Ctrl-C, `SIGTERM`, and `SIGHUP` reach your program and
the whole process tree it spawned, so `envseal run -- docker compose up` stops
cleanly.

### Variable precedence

By default the child sees the parent environment *plus* the decrypted variables,
and the decrypted values win on conflict:

```bash
API_KEY=from-shell envseal run -- printenv API_KEY   # prints the sealed value
```

To withhold the parent environment entirely, pass `--isolated`. The child then
receives only the decrypted variables plus the handful needed to start a program
at all (`PATH`, `HOME`, `TMPDIR`, and the Windows equivalents):

```bash
envseal run --isolated -- ./server
```

Use it when you want certainty that a build or test run cannot pick up a stray
variable from your shell.

### When you really need a file

Some tools insist on reading a `.env` from disk:

```bash
envseal decrypt              # .env.enc → .env
```

The output is written with owner-only permissions. An existing file is never
overwritten unless you pass `--force`, so a stray `decrypt` cannot silently
destroy local edits. Delete the plaintext when you are finished.

To pipe it somewhere instead:

```bash
envseal decrypt -o - > /tmp/env
envseal decrypt -o - | grep DATABASE_URL
```

Printing secrets straight to a terminal is refused, because they would sit in
your scrollback and your terminal's buffer long after the command finished. If
you truly want that, `--force` allows it.

---

## Sharing through Git

`encrypt` and `decrypt` operate on files. `push` and `pull` operate on your
**team**: they drive Git for you, so sharing an environment change is one
command instead of four.

```bash
envseal push        # encrypt → git commit → git push
envseal pull        # git pull → decrypt → summarize what changed
```

### push

```
$ envseal push
Encrypted .env → .env.enc

Will commit .env.enc and push to origin/main.
Continue? [y/N] y
Committed "Update environment"
Pushed to origin/main
```

It asks first, because pushing reaches a remote and cannot be quietly undone.
Pass `--yes` to skip the prompt in a script. Without a terminal **and** without
`--yes`, envseal refuses rather than assuming consent — it will never push
because nobody was there to say no.

| Flag | Effect |
|---|---|
| `-m <msg>` | Commit message (default `Update environment`) |
| `-y`, `--yes` | Skip confirmation |
| `--no-push` | Commit locally, don't push |
| `--no-commit` | Encrypt and stage only |

**What push touches.** The commit contains only `.env.enc` and `.envseal.yaml`.
Git pushes whole branches, though, so any other commits waiting on your branch
go with it — there is no way to push one commit without its ancestors. Envseal
cannot change that, so it discloses it instead:

```
Will commit .env.enc and push to origin/main.

This also pushes 1 other commit already on this branch:
  2fe2b62 Another unrelated commit
Continue? [y/N]
```

**What push refuses to do:**

- **Continues nothing if your plaintext `.env` is tracked by Git.** This is the
  disaster the tool exists to prevent, so it stops before encrypting and tells
  you to `git rm --cached .env` — and to treat those values as exposed, because
  they are already in the repository history.
- **Commits only `.env.enc` and `.envseal.yaml`.** If you have other work
  staged, it stays staged. A secrets commit must never carry someone's
  half-finished changes.
- **Refuses on a detached HEAD or a branch with no upstream**, naming the exact
  `git push -u` command to run.
- **Refuses a merge conflict** in the encrypted file, explaining that ciphertext
  cannot be line-merged.

If the environment has not changed, push says so and creates no commit. This
matters more than it sounds: age randomizes every encryption, so re-encrypting
identical content produces a *different* file. Without that check, every push
would add a meaningless whole-file diff to your history.

### pull

```
$ envseal pull
From github.com/you/project
   c9d5faf..0f40205  main -> origin/main
Decrypted origin/main:.env.enc → .env

+ STRIPE_WEBHOOK_SECRET
~ DATABASE_URL
- OLD_API_KEY
```

The summary is the useful part: you can see that a teammate added one variable
and changed another **without either of you exposing a value**.

**Pull changes nothing but your `.env`.** It fetches, then reads the encrypted
file straight out of the upstream branch — it never checks anything out. Your
branch does not move, no tracked file changes, and nothing is staged:

```
$ git status --porcelain     # before
 M app.txt
$ envseal pull
$ git status --porcelain     # after: identical
 M app.txt
```

Because nothing is checked out, **this works with a dirty working tree**, where
`git pull` would refuse. Getting the current secrets and advancing your branch
are separate concerns; run `git pull` yourself when you want the rest of the
repository.

| Flag | Effect |
|---|---|
| `-f`, `--force` | Overwrite a locally modified `.env` |
| `--no-git` | Decrypt without fetching first |

If your `.env` has been edited since the last sync, pull stops rather than
discarding your work:

```
Error: .env has local changes

It is not what envseal last wrote, so overwriting it would discard your edits.

Check:
  • run `envseal push` to share your changes first
  • pass --force to discard them
```

To tell an edited file from a merely stale one, envseal records a SHA-256 of
each `.env` it writes under `~/.envseal/sync/`. The fingerprint lives in your
home directory, never in the repository, and it is a hash rather than the
content: enough to recognise a file envseal produced, useless for recovering
what was in it.

Pull also refuses to take the upstream copy when you have **committed** an
environment change that is not pushed yet, since that would quietly undo it.

### Granting access with push

Adding a recipient and re-encrypting is two commands:

```bash
envseal add alice age1lggy...
envseal push
```

`push` notices that `.envseal.yaml` has uncommitted changes and re-encrypts for
the new list, even when the environment values themselves are unchanged. Use
`envseal reseal` when you want to re-encrypt without touching `.env` at all.

### Outside a repository

Both commands degrade gracefully. With no Git repository they behave as
`encrypt` and `decrypt`, and say so.

---

## Reviewing and validating

Two commands answer questions about a sealed environment without opening it to
anyone watching.

### `envseal diff`

What has changed, by name:

```
$ envseal diff
+ STRIPE_WEBHOOK_SECRET
~ DATABASE_URL
- OLD_API_KEY

3 changes
```

`+` added, `~` value changed, `-` removed. Unchanged variables are not listed,
and **no value is ever printed** — that is the entire point. A reviewer can see
that a commit rotates `DATABASE_URL` and adds a webhook secret without being
able to read either.

With no arguments it compares your working `.env` against the encrypted file,
which answers "what have I changed that is not sealed yet". To compare against
another revision instead:

```bash
envseal diff --ref HEAD           # what does my working copy change?
envseal diff --ref origin/main    # what does my branch change?
```

| Flag | Effect |
|---|---|
| `--ref <rev>` | Compare the encrypted file against a git revision |
| `--json` | `{"added": [...], "changed": [...], "removed": [...]}` |
| `--exit-code` | Exit 1 when anything differs, for scripts and hooks |

`--exit-code` makes a useful guard: `envseal diff --exit-code` fails when your
`.env` has drifted from `.env.enc`, so you notice before committing.

### `envseal check`

Validates the project and, most importantly, catches plaintext that git can see:

```
$ envseal check
ok   configuration   2 recipients
ok   encrypted file  7 variables
ok   schema          every variable in .env.example is present
FAIL plaintext       committed to the repository, so the values are exposed
    .env.production

1 problem found.
```

Four checks:

1. **configuration** — `.envseal.yaml` parses and lists at least one recipient.
2. **encrypted file** — it exists and opens with your identity.
3. **schema** — every variable named in `.env.example` is present in the
   environment. Extra variables are fine; only missing ones are reported, since
   an environment usually has more than the example documents.
4. **plaintext** — no unencrypted environment file is **tracked by git**
   (a failure: the values are already exposed) or merely **unignored**
   (a warning: one `git add .` from becoming a failure).

`.env.example`, `.env.sample`, `.env.template`, `.env.dist` and anything ending
in `.enc` are recognised as safe and never reported.

A check that cannot run reports **skipped**, not failed — no identity in this
environment, no git repository, no example file. Skipped is not a problem, so
`check` still exits 0.

| Flag | Effect |
|---|---|
| `--strict` | Treat warnings as failures |
| `--schema <file>` | Use a different file as the list of expected variables |
| `--json` | Machine-readable findings |

Exits 1 when anything failed, which is what makes it useful in CI:

```yaml
- name: Validate the environment
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal check --strict
```

Without an identity the schema and decryption checks are skipped, but the
plaintext detection still runs — so even a job with no secrets can catch a
committed `.env`.

---

## Changing a secret

For one variable, `rotate` does it without plaintext ever reaching the disk:

```bash
$ envseal rotate STRIPE_SECRET_KEY
New value for STRIPE_SECRET_KEY:
Read 32 characters.
Rotated STRIPE_SECRET_KEY in .env.enc for 3 recipients.
Updated .env to match.
```

The value is typed without echo. Envseal decrypts in memory, replaces that one
value, and encrypts again. **Every other line survives byte for byte** —
comments, blank lines, ordering, and the quoting of other variables.

| Way to supply the value | When |
|---|---|
| typed at the prompt | the default; nothing is echoed or stored |
| `--stdin` | scripts: `pass show api \| envseal rotate API_KEY --stdin` |
| `--generate` | tokens you never need to see; `--length` sets the size |

There is deliberately **no `--value` flag**. A secret passed as an argument is
written to your shell history and is visible in `ps` to every user on the
machine, which would undo the point of the command.

Rotating a name that does not exist is refused, since it is nearly always a
typo; pass `--add` when you mean it.

If a local `.env` exists and matches the sealed environment, it is updated too.
Leaving it stale would be worse: the next `envseal push` would encrypt the old
value and quietly undo the rotation.

### Changing several at once

```bash
envseal decrypt          # .env.enc → .env
$EDITOR .env             # change what you need
envseal encrypt .env     # .env → .env.enc
rm .env                  # remove the plaintext
```

`encrypt` parses the file before encrypting it. A typo fails immediately with
the line number rather than becoming ciphertext whose problems surface later,
inside a container, at 3am.

---

## Working with other people

The rule to internalise: **a person's access is decided when the file is
encrypted, not when they clone the repository.**

### Adding a teammate

Alice runs `envseal keys generate` on her own machine, then `envseal keys public`
and sends you the line it prints — Slack, email, a pull request, anywhere. It is
not a secret.

You authorize it:

```bash
envseal add alice age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
```

```
Added alice to .envseal.yaml

Run `envseal reseal` to give them access to the current secrets.
```

`add` only edits the configuration. The encrypted file still holds the old
recipient list, so Alice cannot read anything yet. Grant the access:

```bash
envseal reseal
```

```
Re-encrypted .env.enc for 2 recipient(s): you, alice
```

Commit both files. Alice clones, runs `envseal keys generate` if she has not
already, and is immediately productive:

```bash
git pull
envseal run -- ./server
```

This two-step design is deliberate. Adding someone to a config file and silently
re-encrypting every secret in the repository should not be the same keystroke.

### Onboarding checklist

For the new person:

```bash
envseal keys generate        # once per machine
envseal keys public          # send this line to someone on the project
# wait for them to add you and rotate
git pull
envseal status               # Local access should show ✓
envseal run -- ./server
```

---

## Removing someone

```bash
envseal remove alice
envseal reseal
```

```
Removed alice from .envseal.yaml

Run `envseal reseal` to re-encrypt without them.
Anyone holding an older copy of the encrypted file can still read it, so rotate the secrets themselves.
```

That last line is the part people skip, so it is worth stating plainly:

> **Removing a recipient does not un-share what they already have.** If Alice
> ever cloned the repository, she holds a copy of the old `.env.enc` and her key
> still opens it. Git history holds that file forever, too.
>
> Rotating recipients controls **future** encryptions. To actually revoke access
> to a secret, you must change the secret at its source: issue a new API key,
> reset the database password, roll the token. Then encrypt the new values.

Envseal cannot do that part for you. No tool of this kind can.

If you remove your own key, envseal warns you, because after the next rotation
you will not be able to decrypt the project yourself.

---

## Multiple environments

Use one encrypted file per environment:

```
.env.development.enc
.env.staging.enc
.env.production.enc
```

Create them by naming the output:

```bash
envseal encrypt .env.production -o .env.production.enc
```

And use them by naming the input:

```bash
envseal run .env.production.enc -- ./server
envseal decrypt .env.staging.enc          # → .env.staging
```

`.envseal.yaml` names one default file (`file:`), which is what commands use
when you do not pass a path. Point it at whichever file is the everyday one,
usually development.

Recipients are shared across all of them. If production should be readable by
fewer people, keep it in a separate repository or directory with its own
`.envseal.yaml` — envseal finds the nearest configuration by walking up from
your working directory, stopping at a repository boundary.

---

## Docker and Compose

Envseal needs no Docker integration. It sets environment variables in the
process it starts, and Compose reads them:

```bash
envseal run -- docker compose up
```

**There is a subtlety worth understanding**, because it trips people up.

Variables in the process environment are used by Compose for **interpolation**
in `compose.yaml`:

```yaml
services:
  api:
    image: myapp
    environment:
      DATABASE_URL: ${DATABASE_URL}   # interpolated from envseal's environment
```

But they are **not** automatically forwarded into containers. To pass a variable
through by name, list it without a value:

```yaml
services:
  api:
    image: myapp
    environment:
      - DATABASE_URL       # passed through from the host process
      - API_KEY
```

Both forms work with `envseal run`. What does *not* work is expecting a
container to inherit your whole shell environment — Compose has never done that.

For a plain `docker run`, pass variables explicitly:

```bash
envseal run -- docker run --rm -e DATABASE_URL -e API_KEY myapp
```

Note that any of these expose the secret to anyone who can inspect the container
or run `docker inspect`. That is a property of Docker, not of envseal.

---

## Continuous integration

CI needs a private identity, which means your CI provider holds a secret that
can decrypt your environment. Treat that as the trade it is.

See [ci.md](ci.md) for GitHub Actions, GitLab CI, and other platforms. The short
version:

```yaml
- name: Run tests
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal run -- go test ./...
```

`ENVSEAL_IDENTITY` accepts either a **path** to an identity file or the **key
material itself**, which is what makes the one-liner above work with no
temporary files.

Generate a dedicated CI identity rather than reusing a personal one:

```bash
envseal keys generate --identity ./ci-identity
envseal add ci "$(grep -o 'age1[a-z0-9]*' ci-identity)"
envseal reseal
# paste the contents of ./ci-identity into your CI secret store, then:
rm ci-identity
```

A dedicated key can be revoked without disrupting a human.

---

## Scripting envseal

`envseal status --json` gives machine-readable state and never prints a secret
value:

```bash
envseal status --json
```

```json
{
  "configuration": ".envseal.yaml",
  "configuration_found": true,
  "encrypted_file": ".env.enc",
  "encrypted_file_found": true,
  "identity": "~/.envseal/identity",
  "identity_available": true,
  "recipients": 2,
  "recipient_names": ["you", "alice"],
  "decryptable": true
}
```

`decryptable` is measured by actually attempting a decryption, not by looking
for your key in the recipient list. It answers the real question: *can this
machine read this file right now?*

A useful precondition in a script:

```bash
if [ "$(envseal status --json | jq -r .decryptable)" != "true" ]; then
  echo "No access to this project's secrets." >&2
  exit 1
fi
```

Use `--quiet` to silence progress messages while keeping errors:

```bash
envseal --quiet encrypt .env
```

`--quiet` never suppresses actual output: `decrypt -o -` and `status --json`
still write what you asked for.

---

## Shell completion

```bash
envseal completion bash > /etc/bash_completion.d/envseal
envseal completion zsh > "${fpath[1]}/_envseal"
envseal completion fish > ~/.config/fish/completions/envseal.fish
envseal completion powershell | Out-String | Invoke-Expression
```

---

## The .env format

Envseal parses dotenv files with these rules. When in doubt, quote.

```env
# A comment line.

SIMPLE=value
WITH_SPACES=hello world
EMPTY=
export EXPORTED=value          # the export prefix is accepted and ignored
SPACED = value                 # spaces around = are trimmed

QUOTED="double quoted"
LITERAL='single quoted'
PADDED="  spaces are kept  "

ESCAPES="line1\nline2\ttabbed"     # \n \r \t \\ \" \' \$ in double quotes
NO_ESCAPES='backslash \n stays'    # single quotes are literal

MULTILINE="-----BEGIN KEY-----
abcdef
-----END KEY-----"

DSN=postgres://user:pass@host/db?x=1   # = inside a value is fine
TAG=#not-a-comment                     # no space before #, so it is a value
NOTE=value  # this trailing comment is stripped
```

Things to know:

- **No variable interpolation.** `B=${A}` is the literal text `${A}`. Expanding
  variables is your application's job; doing it here would change what your
  program sees compared to reading the file itself.
- **Inline comments are stripped** when a `#` is preceded by whitespace, which
  matches Docker Compose and godotenv. A `#` inside an unquoted value would be
  cut, so **quote any value containing `#`**.
- **Duplicate keys:** the last assignment wins, as in a shell.
- **CRLF and a UTF-8 BOM** are handled, so files written on Windows work.
- **Rejected loudly:** a line with no `=`, an empty name, a name containing
  spaces or control characters, an unterminated quote, text after a closing
  quote, and NUL bytes in a value. Silently dropping a malformed line would mean
  a variable your app needs is simply missing at runtime.

Errors report the line number and the variable name — never the value.

Encryption preserves your file **byte for byte**, including comments, blank
lines, and ordering. Decrypting returns exactly what you encrypted.

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | General error (bad usage, refused overwrite, missing `--`) |
| `2` | Invalid configuration (`.envseal.yaml` or a malformed `.env`) |
| `3` | Encryption or decryption failure |
| `4` | Identity problem (missing, unreadable, invalid) |
| `5` | Could not execute the child process |
| `6` | Git operation failed (push, pull, commit) |

With `envseal run`, a successful launch replaces this with **the child's own
exit code**, so `envseal run -- sh -c 'exit 7'` exits 7. A child killed by a
signal reports `128 + signal number`, as a shell does.

---

## Troubleshooting

### `unable to decrypt .env.enc`

```
The file was not encrypted for the identity you are using.
```

Your key was not a recipient when the file was last encrypted. Either you were
never added, or someone added you but did not run `envseal reseal`. Check with
`envseal status`, then ask a current recipient:

```bash
envseal add you age1yourkey...
envseal reseal
git commit -am "Grant access"
```

If you expected access, confirm you are using the right identity — `--identity`
and `ENVSEAL_IDENTITY` both override the default.

### `no identity at ~/.envseal/identity`

Run `envseal keys generate`. If your identity lives elsewhere, point at it:

```bash
export ENVSEAL_IDENTITY=/path/to/identity
```

### `not an envseal project`

No `.envseal.yaml` was found in this directory or any parent, stopping at the
repository root. Either you are in the wrong directory, or the project has not
been set up: `envseal encrypt .env`.

### `.env already exists`

`decrypt` will not overwrite a file. Use `--force` to replace it, or `-o` to
write elsewhere.

### `refusing to print secrets to a terminal`

`decrypt -o -` writes to standard output, and you are looking at a terminal.
Redirect it (`> .env`), pipe it, or pass `--force` if you genuinely want the
values on screen.

### `no recipients in .envseal.yaml`

The recipient list is empty, so there is nobody to encrypt for. Add someone:

```bash
envseal add you age1yourkey...
```

### `ENVSEAL_IDENTITY already holds an identity`

You ran `envseal keys generate` while the variable contained key material.
The file would be written and then ignored, because the variable takes
precedence. Unset it, or choose an explicit path with `--identity`.

### `is readable by other users (mode 0644)`

Your identity file is too permissive. Fix it:

```bash
chmod 600 ~/.envseal/identity
```

Envseal warns rather than refusing, because CI checkouts sometimes have
unavoidable modes.

### A merge conflict in `.env.enc`

Encrypted files cannot be merged line by line — every re-encryption changes the
whole ciphertext. Resolve by picking one side and re-applying the change:

```bash
git checkout --theirs .env.enc      # take the incoming version
envseal decrypt -o .env             # look at it
$EDITOR .env                        # re-apply your change
envseal encrypt .env && rm .env
```

To avoid conflicts, coordinate secret changes the way you would a lockfile
update.

### Lost identity

There is no recovery. Envseal has no server and no escrow, which is the entire
point. If nobody else was a recipient, the secrets in that file are gone;
recreate them from their sources. If someone else was a recipient, they can add
your new public key and rotate.

Keeping at least two recipients on any important file is the practical
safeguard.

---

## Where to next

- [configuration.md](configuration.md) — every `.envseal.yaml` field
- [key-management.md](key-management.md) — identities, rotation, and recovery
- [ci.md](ci.md) — CI platform recipes
- [security.md](security.md) — the threat model, stated honestly
- [architecture.md](architecture.md) — how the code is put together
