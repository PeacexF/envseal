# envseal GitHub Action

Installs [envseal](https://github.com/PeacexF/envseal) and validates a project's
encrypted environment, without a Go toolchain on the runner.

```yaml
- uses: PeacexF/envseal/action@v1.0.1
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
```

That installs the binary, runs `envseal check`, annotates any failure on the
pull request, writes a job summary, and fails the job if something is wrong.

## What it checks

| | |
|---|---|
| configuration | `.envseal.yaml` parses and lists a recipient |
| encrypted file | it exists and opens with the CI identity |
| schema | every variable in `.env.example` is present |
| plaintext | no unencrypted `.env` is tracked by Git or left unignored |

A check that cannot run reports **skipped**, not failed.

## Without a secret

`ENVSEAL_IDENTITY` is optional. Without it, the decrypt and schema checks skip —
but **plaintext detection still runs**, and that is the check that catches a
committed `.env`.

Fork pull requests never receive secrets, so this is the configuration that
works on them:

```yaml
on: [pull_request]

jobs:
  envseal:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: PeacexF/envseal/action@v1.0.1
```

## Running your app with the sealed environment

Set `check: false` to install only, then use `envseal run` yourself:

```yaml
- uses: PeacexF/envseal/action@v1.0.1
  with:
    check: false

- env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal run -- go test ./...
```

## Inputs

| Input | Default | |
|---|---|---|
| `version` | `latest` | Release to install, such as `v1.0.1` |
| `check` | `true` | Run `envseal check` after installing |
| `args` | | Extra arguments for `check`, such as `--strict` |
| `working-directory` | `.` | Directory to run in |
| `fail-on-problems` | `true` | Fail the job on a failed check |

## Outputs

| Output | |
|---|---|
| `version` | The version installed |
| `problems` | Number of failed checks |
| `warnings` | Number of warnings |

## Security

**The identity is not an input, on purpose.** Action inputs appear in the
workflow file and in logs; a private key belongs in `env:` from a secret.

**Downloads are verified.** The archive is checked against the SHA-256 published
with the release before anything is executed. A mismatch aborts the step — this
runs in CI, where a tampered binary would be handed a key that decrypts your
environment.

**Pin the version** (`@v1.0.1` rather than `@main`) so a change to the action
cannot alter what runs in your pipeline without you choosing it.

Giving CI an identity means your CI provider can decrypt your environment. See
[docs/security.md](../docs/security.md).
