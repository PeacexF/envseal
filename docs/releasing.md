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

To enable it:

1. Create a public repository named **`PeacexF/homebrew-tap`**.
2. Create a fine-grained personal access token with **contents: write** on that
   repository only.
3. Add it to this repository as the secret `HOMEBREW_TAP_TOKEN`.

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

## If a release goes wrong

Delete the GitHub release and the tag, fix, and tag again:

```bash
git push --delete origin v1.0.0
git tag -d v1.0.0
```

Do this only for a release nobody has installed yet. Once a version is out,
publish a new patch instead — a checksum that changes under a published version
is indistinguishable from tampering.
