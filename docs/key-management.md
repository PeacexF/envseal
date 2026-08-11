# Key Management

Everything about identities: creating them, moving them, sharing access, taking
it away, and what happens when one is lost.

## What an identity is

An identity is an [age](https://age-encryption.org) X25519 key pair stored in
one file:

```
# created: 2026-08-11T18:33:00Z
# public key: age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
AGE-SECRET-KEY-1QQQQ...
```

The commented public key is convenience; the last line is the secret. The format
is plain `age-keygen` output, so the file works with the `age` CLI directly and
envseal is not a lock-in.

**One identity per person per machine.** Not one per project — the same identity
decrypts every project that lists your public key.

## Creating one

```bash
envseal keys generate
```

Written to `~/.envseal/identity` with mode `0600` inside a `0700` directory.
Envseal prints only the public key.

To keep an identity somewhere else — a second key for a specific purpose, a
removable drive, a CI key you are about to upload:

```bash
envseal keys generate --identity ./ci-identity
```

## Finding your public key

```bash
envseal keys public
```

```
age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
```

It prints one line and nothing else, so it pipes cleanly:

```bash
envseal keys public | pbcopy
```

Your public key is not secret. Post it in a channel, put it in a pull request,
print it on a mug.

## Replacing an identity

```bash
envseal keys generate --force
```

This generates a new key and moves the old one to
`~/.envseal/identity.20260811T183300Z.bak`.

**It does not migrate anything.** Every file encrypted for your old key stays
encrypted for your old key. To move a project to your new identity:

```bash
envseal --identity ~/.envseal/identity.20260811T183300Z.bak decrypt -o .env
envseal add me-new age1yournewkey...
envseal remove me-old
envseal encrypt .env && rm .env
```

Do this for each project before deleting the backup. This is why `--force` keeps
a backup instead of overwriting: destroying a private key is irreversible, and a
mistaken `--force` at the wrong moment would otherwise cost you every secret you
have.

## Granting access

```bash
envseal add alice age1lggyhqrw2nlhcxprm67z43rta597azn8gknawjehu9d9dl0jq3yqqvfafg
envseal reseal
git commit -am "Grant alice access"
```

Two steps, on purpose:

- `add` records the key in `.envseal.yaml`. Nothing is re-encrypted.
- `reseal` decrypts with your identity and re-encrypts for the current list.
  `envseal push` does the same thing when it notices the pending change.

Until then, Alice holds a key that opens nothing.

## Withdrawing access

```bash
envseal remove alice
envseal reseal
git commit -am "Revoke alice"
```

Then read this twice:

> **Rotation does not retract what has already been shared.** Alice's copy of
> the old `.env.enc` still opens with her key, and so does every version in Git
> history. Rotating recipients controls future encryptions only.
>
> To genuinely revoke access to a secret, **change the secret**: issue a new API
> key, reset the password, roll the token. Then encrypt the new value.

Treat `envseal remove` as bookkeeping and the credential rotation as the actual
security action.

## Rotating secret values

The routine after someone leaves, or after any suspected exposure:

1. Change each credential at its source (cloud console, database, provider).
2. Put the new value in, one variable at a time, without writing plaintext:

   ```bash
   envseal rotate STRIPE_SECRET_KEY      # typed, hidden
   envseal rotate SESSION_SECRET --generate
   ```
3. `envseal push` to share them.

Envseal cannot automate step 1, and neither can any tool that does not hold your
provider credentials. Step 2 is the part it makes safe: the environment is
decrypted in memory, one value is replaced, and it is sealed again — the
plaintext never reaches the disk.

## Machine and CI identities

Give every non-human consumer its own identity:

```bash
envseal keys generate --identity ./deploy-identity
envseal add deploy "$(grep -o 'age1[a-z0-9]*' deploy-identity)"
envseal reseal
# store the file contents in your secret manager, then
rm deploy-identity
```

Why a separate key:

- It can be revoked without disturbing a person.
- Its exposure tells you exactly which system leaked.
- Nobody has to share a personal key with a build server.

See [ci.md](ci.md) for how each platform supplies it.

## Keeping two recipients

The single most useful habit: **never leave an important file with only one
recipient.** A second recipient — a teammate, a break-glass key in a password
manager, an offline backup identity — is the only thing standing between a lost
laptop and permanently unreadable secrets.

To create a backup identity:

```bash
envseal keys generate --identity ./backup-identity
envseal add backup "$(grep -o 'age1[a-z0-9]*' backup-identity)"
envseal reseal
# store backup-identity somewhere offline, then remove the local copy
rm backup-identity
```

## Losing an identity

There is no recovery mechanism. No server, no escrow, no reset link — that
absence is the design.

- **Other recipients exist:** any of them adds your new public key and rotates.
  Nothing is lost.
- **You were the only recipient:** the contents of that file are unrecoverable.
  Recreate the secrets from their sources and encrypt them again.

## Protecting the identity file

- Never commit it. Never paste it into a chat, an issue, or a log.
- Keep the mode at `0600`; envseal warns when it is looser.
- Back it up the way you back up a password manager, not the way you back up
  source code.
- Anyone who can read the file has your access. Full-disk encryption and a
  locked screen are part of the threat model — see [security.md](security.md).

## Interoperability with `age`

Envseal writes standard age files, so nothing here is a dead end:

```bash
age -d -i ~/.envseal/identity .env.enc
age -e -a -r age1alice... -o .env.enc .env
```

Envseal always writes ASCII armor and reads both armored and binary age files.
If envseal disappeared tomorrow, `age` alone would still open every file it ever
produced.
