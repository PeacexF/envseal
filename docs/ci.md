# Continuous Integration

Envseal needs no server, so CI support is only ever two things: install the
binary, and give it an identity.

## Before you start

**Use a dedicated CI identity.** Never upload a personal key to a build system.

```bash
envseal keys generate --identity ./ci-identity
envseal add ci "$(grep -o 'age1[a-z0-9]*' ci-identity)"
envseal reseal
cat ci-identity          # paste this into your CI secret store
rm ci-identity           # then delete the local copy
git commit -am "Authorize CI"
```

A dedicated key can be revoked without disturbing anyone, and if it leaks you
know exactly which system leaked it.

**Understand what you are accepting.** Your CI provider now holds a key that
decrypts your environment. Anyone who can edit a workflow file, open a pull
request that runs one, or read the provider's secret store can potentially reach
those secrets. That is inherent to running your code on someone else's machine —
see [security.md](security.md).

## `ENVSEAL_IDENTITY`

Envseal reads the identity from `ENVSEAL_IDENTITY`, which accepts **either** a
file path **or** the key material itself. CI secrets are environment variables,
so the second form means you never write the key to a file:

```yaml
env:
  ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
```

Surrounding whitespace is trimmed, so a trailing newline from a copy-paste is
harmless.

---

## GitHub Actions

The quickest route is the action, which installs a checksum-verified binary and
validates the project:

```yaml
- uses: PeacexF/envseal/action@v1.0.1
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
```

It annotates failures on the pull request and writes a job summary. Without the
secret it still runs, and still catches a committed plaintext `.env` — which is
what makes it useful on fork pull requests. See [the action's README](../action/README.md).

To run your own commands, install only:

```yaml
- uses: PeacexF/envseal/action@v1.0.1
  with:
    check: false
- env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal run -- go test ./...
```

The rest of this section sets it up by hand, which is what the action does
internally. Store the contents of `ci-identity` as a repository secret named
`ENVSEAL_IDENTITY`.

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Install envseal
        run: go install github.com/PeacexF/envseal/cmd/envseal@latest

      - name: Run tests with the sealed environment
        env:
          ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
        run: envseal run -- go test ./...
```

**Pull requests from forks do not receive secrets**, which is a GitHub security
feature, not a bug. A fork PR will fail with exit 4 (no identity). Either skip
the sealed steps for forks, or use `pull_request_target` with a full
understanding of what that opens up.

Validate the environment on every pull request. This job needs no identity for
the plaintext check, which is the one that catches a committed `.env`:

```yaml
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/PeacexF/envseal/cmd/envseal@latest
      - env:
          ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
        run: envseal check --strict
```

Verify access before a long job, so failures are legible:

```yaml
- name: Check access
  env:
    ENVSEAL_IDENTITY: ${{ secrets.ENVSEAL_IDENTITY }}
  run: envseal status --json
```

---

## GitLab CI

Store the key as a **masked, protected** CI/CD variable. A `File`-type variable
also works, since `ENVSEAL_IDENTITY` accepts a path — GitLab sets the variable
to the temporary file's path automatically.

```yaml
test:
  image: golang:1.26
  variables:
    ENVSEAL_IDENTITY: $ENVSEAL_IDENTITY   # masked variable, or File type
  before_script:
    - go install github.com/PeacexF/envseal/cmd/envseal@latest
  script:
    - envseal run -- go test ./...
```

Masking requires a single-line value; age private keys qualify. If you paste the
whole identity file (with its comment lines), use a `File` variable instead.

---

## CircleCI

```yaml
version: 2.1

jobs:
  test:
    docker:
      - image: cimg/go:1.26
    steps:
      - checkout
      - run: go install github.com/PeacexF/envseal/cmd/envseal@latest
      - run:
          name: Test
          command: envseal run -- go test ./...
```

Set `ENVSEAL_IDENTITY` as a project environment variable or in a context.

---

## Jenkins

```groovy
pipeline {
  agent any
  stages {
    stage('Test') {
      steps {
        withCredentials([string(credentialsId: 'envseal-identity',
                                variable: 'ENVSEAL_IDENTITY')]) {
          sh 'envseal run -- go test ./...'
        }
      }
    }
  }
}
```

---

## Docker builds

**Do not bake secrets into an image.** Layers are readable by anyone who pulls
it, including deleted files from earlier layers.

Supply the environment at *run* time instead:

```bash
envseal run -- docker run --rm -e DATABASE_URL -e API_KEY myapp
```

If a build genuinely needs a secret, use BuildKit secret mounts, which leave
nothing in the image:

```bash
envseal run -- sh -c 'DOCKER_BUILDKIT=1 docker build \
  --secret id=api_key,env=API_KEY -t myapp .'
```

```dockerfile
RUN --mount=type=secret,id=api_key \
    API_KEY="$(cat /run/secrets/api_key)" ./build.sh
```

---

## Deployment

The same pattern works wherever your process starts:

```bash
envseal run .env.production.enc -- ./server
```

As a systemd unit:

```ini
[Service]
Environment=ENVSEAL_IDENTITY=/etc/envseal/identity
ExecStart=/usr/local/bin/envseal run /srv/app/.env.production.enc -- /srv/app/server
```

Keep `/etc/envseal/identity` at mode `0600`, owned by the service user. Signals
and exit codes pass through envseal, so systemd's restart and stop handling
behaves exactly as it would without it.

---

## Troubleshooting CI

**Exit 4, `no identity`** — the secret is not reaching the step. Check the
variable name, and remember that fork pull requests get no secrets.

**Exit 3, `not encrypted for the identity you are using`** — the CI key was
added but nobody ran `envseal reseal`, or the rotation was never committed.
Run `envseal status --json` locally and confirm the CI key is in
`recipient_names`, then check that `.env.enc` was committed after the rotate.

**Exit 2, `not an envseal project`** — the job's working directory is not inside
the repository, or the checkout is shallow in a way that omitted the config.

**Secrets appearing in logs** — envseal never prints values, but your program
might. Check that your application does not log its own configuration at
startup, and rely on the provider's log masking as a second line of defence, not
the first.
