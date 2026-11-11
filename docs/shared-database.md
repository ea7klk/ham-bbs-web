# Sharing the SQLite database

Both applications must see the same file at the same in-container path:

```text
/var/lib/bbs/bbs.sqlite
```

The Fleet deployment does that with one persistent volume claim mounted into the
`bbs` and `bbs-web` containers. The Dockge compose file does it by bind-mounting
one host directory into both containers:

```yaml
volumes:
  - ./data/bbs:/var/lib/bbs
environment:
  - BBS_DB_FILE=/var/lib/bbs/bbs.sqlite
```

The original SSH BBS and this web app both open SQLite through GORM with:

```text
?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate
```

`_txlock=immediate` makes every transaction take the write lock at `BEGIN`
instead of at its first write, and `_busy_timeout` makes a waiting writer block
rather than fail. Together they serialise read-then-write transactions across
the two applications instead of letting them interleave into a lost update.
Keep the database, `bbs.sqlite-wal`, and `bbs.sqlite-shm` files together when
backing up or moving a live system.

## The shared contract

A change to any of these must be made in both repositories, in the same release
window, or one surface will write rows the other cannot use:

- **Schema.** `dbUser`, `dbBulletin`, `dbBoard`, `dbMessage`, `dbAPRSSent`,
  `dbAPRSSentPart`, `dbAPRSReceived`, `dbActivation`, `dbToken`,
  `dbRegistration` and `dbSetting` are declared identically in
  `simple-ham-bbs/bbs/cmd/bbs/` and in `models.go` here. Both applications read
  and write all of them: a code or link issued by one is redeemed by the other.
  `dbAuthFailure` (`db_auth_failures`) is the exception — it is the SSH BBS's
  own cross-process counter, and the web app keeps its throttle in memory.
- **Account tokens.** A link token is `base64.RawURLEncoding` of 32 random bytes;
  the database holds only `sha256` hex of it. An activation code is four
  characters from `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`, stored as
  `sha256(callsign + "|" + code)`. Purposes are `password_reset`,
  `email_change_old` and `email_change_new`; an email change pairs two tokens
  under one `change_id` and applies only when both are consumed. TTLs: 2 hours
  for a reset, 24 hours for a code or an email change.
- **Registration metering.** `db_registrations` counts accounts per client
  address: five per hour, in both applications.
- **Password format.** `pbkdf2_sha256$iterations$salt$digest` with 200,000
  iterations, base64 salt and digest.
- **Reply nesting.** `maxReplyDepth = 5` on both sides. The SSH BBS enforces it
  in `createMessageRow`, the web app in `createMessage`; neither may insert a
  message one level deeper.
- **Text sanitising.** `sanitizeSingleLineText` / `sanitizeMultilineText` use
  the same rule in both repositories. Text is cleaned on write and again on
  read.
- **Sysop bootstrap.** `BBS_SYSOPS` grants the role only while no enabled sysop
  exists. `isSysop` answers from the stored row alone on both sides.
- **Timestamps.** `2006-01-02 15:04 UTC` strings, written by `now()`.
- **Registration date.** `dbUser.Registered` is written once, at account
  creation, and never updated afterwards — unlike `FirstSeen`/`LastSeen`, which
  both move. `Registered` and `FirstSeen` are set to the same stamp when the
  account is created. Both applications backfill the column from `first_seen`
  on startup for accounts that predate it, idempotently, and leave rows with no
  `first_seen` empty rather than inventing a date. Do not "refresh" it on a
  profile edit, a login, or a sysop change.

## Who runs what

The classic SSH BBS remains the APRS worker. It receives APRS messages, sends
APRS ACKs for received messages, receives ACKs/rejections for sent messages, and
updates the shared SQLite tables. The web image only reads the shared APRS
tables and can add sent-message rows when a user sends from the web UI; it does
not run an APRS receiver or any ACK loop.

## Dockge deployment (self-hosted, outside Fleet)

1. Copy `.env.example` to `.env` and edit the passwords, sysop callsigns, and APRS callsigns.
2. Put the real WireGuard profile at `hamnet/wg_confs/wg0.conf` if you use the HamNet container.
3. In Dockge, create a stack from `compose.dockge.yaml`.
4. Start the stack.

The SSH BBS is available on `${BBS_SSH_PORT:-2222}`. The web app listens on
container port `8080` and is intended to be exposed by your Dockge/Traefik
deployment. Both BBS applications use `./data/bbs/bbs.sqlite` on the Docker host.

The production deployment for `spainip-k3s` is not Dockge: it is the `bbs` and
`bbs-web` Deployments in `spainip/infra` → `fleet/managed-apps.yaml`, reconciled
by Fleet from a digest-pinned image. See the repository README.

## If you already have BBS data

Stop the existing BBS container first, then copy the whole data directory into
this stack:

```sh
mkdir -p data/bbs
cp /path/to/current/bbs.sqlite* data/bbs/
```

Start the stack after the copy. Do not copy only `bbs.sqlite` while the old
service is running, because WAL mode can keep recent writes in `bbs.sqlite-wal`.
