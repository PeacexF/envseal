# Releasing

For maintainers. Cutting a release is one command; everything else is
automated.

## Cut a release

```bash
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

Pushing a `v*` tag runs [`.github/workflows/release.yml`](../.github/workflows/release.yml),
which re-runs the tests against the tagged commit and then builds and publishes.
A failing test stops the release before anything is uploaded.

The result:

| Artifact | |
|---|---|
| `envseal_<version>_<os>_<arch>.tar.gz` | linux and macOS, amd64 and arm64 |
| `envseal_<version>_<os>_<arch>.zip` | windows, amd64 and arm64 |
| `checksums.txt` | SHA-256 of every archive |

Each archive carries the binary, `README.md`, `LICENSE`, `SECURITY.md`, and the
`docs/` directory, so the documentation matches the binary it shipped with.

Binaries are statically linked (`CGO_ENABLED=0`), stripped, built with
`-trimpath`, and stamped with the tag:

```bash
$ envseal --version
envseal v1.0.0
```

`mod_timestamp` is taken from the commit rather than the clock, so rebuilding
the same tag produces the same bytes.

## Versioning

Tags are `vMAJOR.MINOR.PATCH`. A tag containing a hyphen (`v1.0.0-rc1`) is
published as a prerelease automatically.

**Exit codes are public API.** `0` success, `1` general, `2` configuration,
`3` encryption, `4` identity, `5` process, `6` git. Scripts depend on them, so
changing one is a major version.

The same applies to the `--json` shapes of `status`, `diff`, and `check`, and to
the `.envseal.yaml` `version` field: a format change means a new config version
that older binaries refuse rather than misread.

## Homebrew

Publishing to a tap is optional. Without it the release still succeeds and only
the Homebrew step is skipped, so this can wait until you want it.

### Create the tap

Homebrew scaffolds it for you — no clicking through GitHub. See
[How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap).

```bash
brew tap-new PeacexF/homebrew-tap
cd "$(brew --repository)/Library/Taps/peacexf/homebrew-tap"
gh repo create PeacexF/homebrew-tap --public --source=. --push
```

The `homebrew-` prefix is what makes the short `brew install PeacexF/tap/envseal`
form work.

`tap-new` writes a README, a `Formula/` directory, and three workflows aimed at
source-built formulae. Envseal ships prebuilt binaries, so GoReleaser publishes a
**cask** into `Casks/` (which it creates on the first release) and `Formula/`
stays empty. Two of the scaffolded workflows do not apply:

```bash
rm .github/workflows/publish.yml    # bottle publishing: casks have no bottles
rm .github/workflows/autobump.yml   # GoReleaser bumps the version itself
git commit -am "Remove formula-only workflows" && git push
```

`tests.yml` needs replacing rather than keeping. It runs
`brew test-bot --only-tap-syntax`, which includes `brew style` — and GoReleaser's
generated cask does not satisfy Homebrew's layout rules: it emits `on_intel`
before `on_arm`, with blank lines Homebrew wants closed up. That is six
autocorrectable offences on **every** release, and a tap-level `.rubocop.yml`
does not silence them (verified: `brew style` ignores it for casks).

The other two checks in that step are worth keeping, so run them directly:

```yaml
name: tests

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: Homebrew/actions/setup-homebrew@master
      - run: brew tap PeacexF/tap
      # Deliberately not `brew test-bot --only-tap-syntax`: it runs `brew style`,
      # which lints a file GoReleaser generates and we cannot reformat.
      - run: brew readall --aliases --os=all --arch=all peacexf/tap
      - run: brew audit --except=installed --tap=peacexf/tap
```

`readall` catches a cask that fails to load and `audit` catches a bad URL or
description — the failures that would actually break an install. Only the
layout check is dropped.

### Give the release a token

1. GitHub → Settings → Developer settings → **Fine-grained tokens**.
2. Repository access: **only** `PeacexF/homebrew-tap`.
3. Permissions: **Contents → Read and write**. Nothing else.
4. Add it to `PeacexF/envseal` as the Actions secret `HOMEBREW_TAP_TOKEN`.

Scope it to the tap alone. It is a write credential living in a CI system, which
is the same trade [security.md](security.md) describes for CI identities: keep
the blast radius to one repository that contains nothing secret.

The next release writes `Casks/envseal.rb` there, and installation becomes:

```bash
brew install PeacexF/tap/envseal
```

The cask installs shell completions and clears the macOS quarantine flag, so the
first run is not blocked by Gatekeeper.

## Testing the pipeline without releasing

```bash
goreleaser check                                   # validate the config
goreleaser release --snapshot --clean --skip=publish
```

The second builds every platform into `dist/` and publishes nothing. Worth
running after any change to `.goreleaser.yaml`, because a config can validate
and still fail at release time — the tap-token check did exactly that.

## Checklist

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] Fuzzing has been run recently: `go test -fuzz FuzzParse -fuzztime 60s ./internal/dotenv/`
- [ ] Documentation matches the behaviour being shipped
- [ ] `goreleaser release --snapshot --clean --skip=publish` succeeds
- [ ] Tag and push

## Troubleshooting

**`homebrew cask: expected {{ .Env.VAR_NAME }} only (no plain-text or other
interpolation)`**

The `token` field accepts *only* the exact form `{{ .Env.NAME }}`. Anything
else — a function call, a default, surrounding text — is rejected when the cask
is published. `skip_upload` has no such rule, which is why it can use the
tolerant `{{ if index .Env "..." }}` form to keep local snapshots working while
`token` stays strict.

This is not caught by `goreleaser check` or by a snapshot, because publishing is
skipped in both. Only a real tag exercises it.

**The release published but a later step failed.** GoReleaser has no way to skip
only the GitHub release and re-run a later publisher (`--skip` takes whole
steps: `homebrew`, `publish`, `announce`, …). Fix the config and tag a new patch
version; do not re-tag the published one.

## If a release goes wrong

Delete the GitHub release and the tag, fix, and tag again:

```bash
git push --delete origin v1.0.0
git tag -d v1.0.0
```

Do this only for a release nobody has installed yet. Once a version is out,
publish a new patch instead — a checksum that changes under a published version
is indistinguishable from tampering.
