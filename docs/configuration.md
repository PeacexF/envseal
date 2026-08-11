# Configuration

Envseal is configured by one file per project, plus a flag and an environment
variable for the identity. There is nothing else: no global config, no profiles,
no state directory beyond `~/.envseal`.

## `.envseal.yaml`

Lives at the project root, next to the encrypted file. **Commit it.** It holds
public information only.

```yaml
# envseal project configuration
# Public information only. Never put secret values in this file.

version: 1
file: .env.enc
recipients:
  - name: you
    key: age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
  - name: alice
    key: age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
```

### `version` (required)

Format version of this file. Currently `1`. Envseal refuses a version it does
not understand rather than guessing, so an older binary will not silently
mishandle a newer file.

### `file` (optional)

Default encrypted file for commands run without a path. Defaults to `.env.enc`.

Must be a path **inside the project**. Absolute paths, drive letters, UNC paths,
and `..` segments are rejected on every operating system, not just the one you
are using — a repository is shared between machines, so a path that escapes on
Windows is rejected on Linux too.

```yaml
file: .env.enc            # fine
file: config/.env.enc     # fine
file: /etc/secrets.enc    # rejected
file: ../other/.env.enc   # rejected
```

### `recipients` (required to encrypt)

Who may decrypt. Each entry has a `name` and a public `key`.

```yaml
recipients:
  - name: alice
    key: age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
```

- `name` is a label for humans. It is how you refer to the recipient in
  `envseal remove <name>`. Names must be unique, compared case-insensitively.
- `key` is an age public key, starting with `age1`. Keys must be unique and are
  validated when the file is read, so a typo is caught immediately.

An empty list loads without error but refuses to encrypt. That is deliberate:
if an empty list were fatal at load time, `envseal add` could never repair a
project that had lost its last recipient.

Editing this file by hand is fine. Prefer `envseal add` and `envseal remove`,
which validate as they go.

### Unknown fields are rejected

A misspelled key is an error, not a silently ignored line:

```
Error: invalid configuration in .envseal.yaml

line 3: field recipient not found
```

A typo in a security-relevant file should never be a no-op.

### Comments are not preserved

`envseal add` and `envseal remove` rewrite the file canonically, regenerating
the header comment. Comments you add by hand elsewhere in the file are lost on
the next write. Keep notes in your README.

---

## Identity location

Envseal looks for your private identity in this order. The first one that is set
wins.

| Order | Source | Example |
|---|---|---|
| 1 | `--identity` flag | `envseal --identity ./ci-key decrypt` |
| 2 | `ENVSEAL_IDENTITY` variable | `ENVSEAL_IDENTITY=/path/to/identity envseal run -- ./app` |
| 3 | Default path | `~/.envseal/identity` |

### `ENVSEAL_IDENTITY`

Accepts **either** a path **or** the key material itself. Envseal tells them
apart by the `AGE-SECRET-KEY-` prefix, and trims surrounding whitespace.

```bash
export ENVSEAL_IDENTITY=/media/usb/identity                      # a path
export ENVSEAL_IDENTITY="AGE-SECRET-KEY-1QQQQ..."                # key material
```

The second form exists for CI, where secrets arrive as environment variables and
writing a temporary file would be an unnecessary risk. See [ci.md](ci.md).

Running `envseal init` while `ENVSEAL_IDENTITY` holds key material is refused:
the file would be created and then ignored, because the variable takes
precedence.

---

## Global flags

Available on every command.

| Flag | Effect |
|---|---|
| `--identity <path>` | Use this private identity instead of the default |
| `--quiet`, `-q` | Suppress progress messages; errors and payload still print |
| `--help`, `-h` | Help for any command |
| `--version` | Print the version (root command only) |

`--quiet` silences chatter such as `Encrypted .env → .env.enc`. It never
silences requested output: `decrypt -o -` and `status --json` still write.

---

## Files envseal touches

| Path | Mode | Purpose |
|---|---|---|
| `~/.envseal/` | `0700` | Identity directory |
| `~/.envseal/identity` | `0600` | Your private key |
| `~/.envseal/identity.<timestamp>.bak` | `0600` | Previous key, kept by `init --force` |
| `.envseal.yaml` | `0644` | Project configuration, committed |
| `.env.enc` | `0644` | Encrypted environment, committed |
| `.env` (from `decrypt`) | `0600` | Plaintext, never committed |

Modes are set on Unix. Windows uses its own access control, and envseal skips
the permission checks there.

Every write is atomic: content goes to a temporary file in the destination
directory and is then renamed into place, so a crash cannot leave a half-written
`.env.enc` or a truncated identity.
