# Architecture

How envseal is put together, for anyone changing it.

## Shape

A CLI in front of `age`. The interesting work is not cryptographic — it is
resolving what to encrypt, for whom, and handing the result to a process without
letting it touch a disk.

```
cmd/envseal/main.go        os.Exit(cli.Main(os.Args[1:]))
    │
internal/cli/              command definitions, output, exit codes
    │
    ├── config/            .envseal.yaml
    ├── project/           finding the project root
    ├── identity/          the local private key
    ├── dotenv/            parsing .env
    ├── crypto/            age encryption
    ├── process/           running the child
    ├── safefile/          atomic writes
    ├── errs/              error type and exit codes
    └── version/           build version
```

Dependencies point one way: `cli` uses everything, the leaf packages use only
`errs` and `safefile`. No package imports `cli`.

## Packages

### `errs`

The error type. Every failure carries an exit code, a summary, an optional
explanation, and a list of things to check:

```go
errs.New(errs.CodeCrypto, "unable to decrypt %s", source).
    Detailf("The file was not encrypted for the identity you are using.").
    Check("confirm your public key is listed in .envseal.yaml")
```

`errs.CodeOf(err)` gives the exit code — defaulting to 1 — and `errs.Render`
prints the block. `errs.Exit(code)` is a *silent* error used to carry a child
process's status without printing anything.

The rule that governs this package: **errors carry paths, line numbers, and
names, never values.**

### `safefile`

One function: `Write(path, data, perm)`. Writes to a temporary file in the same
directory at mode `0600`, widens to `perm` after writing, then renames. Every
writer in the project goes through it.

### `config`

`.envseal.yaml`. `Load`, `Parse`, `Save`, plus `ValidateKey` and
`ParseRecipients`. Unknown fields are rejected; `escapesProject` keeps `file:`
inside the project on every platform, not just the current one.

### `project`

Walks up from a directory looking for `.envseal.yaml`, stopping at a `.git`
boundary so a search never escapes into an unrelated project. Returns
`ErrNotFound` wrapped inside a typed error, so callers can either match the
sentinel with `errors.Is` or let the actionable message reach the user.

### `identity`

Generating, loading, and resolving the private key. Resolution order is
`--identity`, then `ENVSEAL_IDENTITY` (path or key material), then
`~/.envseal/identity`.

Private material never escapes: `String()` returns the *public* key, and parse
failures never echo their input.

### `dotenv`

Order-preserving parser. `File` keeps the **original bytes** and `Bytes()`
returns them unchanged, so encryption reproduces the source exactly — there is
no renderer, and therefore no class of reformatting bugs. Parsing exists to
serve `run` and `status`, and to validate before encrypting.

### `crypto`

The age wrapper. `Encrypt` writes ASCII armor; `Decrypt` reads armored or binary
age files. It maps age's errors onto envseal's actionable ones, distinguishing
"not encrypted for you" from "this file is damaged".

### `process`

Runs the child. Puts it in its own process group, forwards signals, and reports
its exit code. Platform differences live in `process_unix.go` and
`process_windows.go` behind `configure`, `forwarded`, `signalChild`, and
`signalExitCode`.

### `cli`

One file per command. `app` holds the global flags; `workspace.go` resolves the
project and configuration once so every command handles paths identically:
**explicit path arguments are relative to your working directory, paths from the
configuration are relative to the project root.**

`display()` shortens paths for output — relative to the working directory, then
`~`, then absolute.

## Command flow

`envseal run`:

```
parse args, split on --
  → workspace(): find project, load config
  → read the encrypted file
  → identity.Resolve(): flag, env, default
  → crypto.Decrypt()
  → dotenv.Parse()
  → compose parent + decrypted environment
  → clear(plaintext)
  → process.Run()
  → errs.Exit(childCode)
```

`envseal encrypt`:

```
dotenv.Load(source)          validate before encrypting
  → resolveRecipients()      --recipient, else config, else bootstrap self
  → config.ParseRecipients()
  → crypto.Encrypt(file.Bytes())
  → safefile.Write(target, 0644)
```

## Conventions

- **Black-box tests only.** Every test file is `package <pkg>_test` and uses the
  exported API. If something needs testing but is unreachable from outside, the
  API is wrong.
- **Sparse comments.** Package docs, non-obvious decisions, and security
  invariants. Nothing that restates the code.
- **`source` parameters.** `Parse` functions take the origin's name for error
  messages rather than reading files themselves.
- **Exit codes are public API.** Scripts depend on them; they do not change.

## Adding a command

1. Create `internal/cli/<name>.go` with `new<Name>Cmd(a *app) *cobra.Command`.
2. Register it in `NewRoot`.
3. Use `a.stdout(cmd)` for progress output so `--quiet` works, and
   `cmd.OutOrStdout()` for payload that must always print.
4. Return `*errs.Error` with the right code.
5. Add `internal/cli/<name>_test.go` in `package cli_test`, driving it through
   `cli.Run` with buffers.

If the command touches secrets, assert they do not appear in its output.

## Testing

```bash
make test          # go test ./...
make check         # vet, tests, gofmt
go test -race ./...
go test -fuzz FuzzParse ./internal/dotenv/
```

CI runs the suite on Linux, macOS, and Windows, plus gofmt and `go mod tidy`
drift checks and `govulncheck`.
