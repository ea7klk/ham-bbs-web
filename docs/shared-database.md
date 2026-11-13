# Sharing the SQLite database

## Which application is canonical

**The classic SSH BBS is canonical. This web app is its companion.**

Both applications read and write one file, but they are not peers, and the
difference decides every question that starts "which one is right?":

- **Data.** If the two ever disagree about what a row says, the SSH BBS's value
  wins. This app must never treat its own view as authoritative over the file the
  SSH BBS is serving.
- **Schema.** Tables, columns, indexes and migrations originate in
  `simple-ham-bbs`. This app's `AutoMigrate` may create what that repository has
  already declared — it must never be the first to introduce a table, a column or
  an index, because the canonical side would then be running against a schema it
  does not know about.
- **Rules.** Reply depth, text sanitising, the sysop bootstrap, token formats, the
  registration meter, the email index and the APRS message sections are all
  defined by the SSH BBS and mirrored here. A change is made there first, then
  here, and this app follows rather than negotiates.
- **Conflicts in review.** When the two disagree and neither is obviously wrong,
  the SSH BBS's behaviour is the specification and this app has the bug.

This is not a comment on which is better. The terminal client is the older,
more complete surface — it owns APRS receive and ACK, activation and the schema —
and a companion that quietly disagreed with it would be worse than no companion.

### What that means in an incident

On 2026-11-12 the ham-bbs volumes were re-provisioned and the database was
replaced. The SSH BBS opens the file per session, so it picked up the replacement
at once; this app opens it once at startup and kept serving the deleted inode for
fifteen hours — reading a database that no longer existed on disk while the
canonical one moved on. Restarting this app was the fix, and it is the general
shape of the failure: **this app must never assume the file it opened is still the
file on disk.** See the reload invariant in `AGENTS.md`.

## The shared file

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
- **Email indexing.** `dbUser.Email` carries
  `gorm:"index:idx_users_email_nocase,collate:nocase"` in both repositories, and
  every lookup by address names the collation: `WHERE email = ? COLLATE NOCASE`.
  SQLite only uses a collated index when the query asks for the collation too, so
  a bare `email = ?` silently scans the table — which is what all email lookups
  did before this index existed. Adding the index to only one repository is
  harmless but pointless: the other's migration would not create it.
- **APRS bulletins.** `db_aprs_bulletins` is written **only** by the classic BBS
  receiver — it parses `BLNn`+group addressees, deduplicates by content, assigns
  the category and enforces the ten-day retention from `last_seen`. The web app
  reads the table and nothing else. If a category is ever added, add it to
  `bulletinCategories` there and to `aprsBulletinCategories` here, plus the
  `aprs_bulletin_category_<id>` and `aprs_bulletin_desc_<id>` strings.
- **Message sections.** Both applications file APRS history into three sections by
  the peer callsign alone: `APRSPH` is the net bot, `ANSRVR` is the group server
  (`#APRSThursday` and any other group), and everything else is traffic with
  another station. The classification lives in `aprs_scope.go` in each repository
  and must stay identical: the two surfaces show the same rows, and a message
  filed differently in each is a bug with no single owner. Nothing is stored — the
  peer is already in the row, so no migration is involved.
- **Bot commands are a wire format.** `APRSPH` is a station; `ANSRVR` is the group
  server. The command strings (`CQ HOTG`, `U HOTG`, `K HOTG`, `LIST`,
  `LAST10 THURS`, `?APRSM`, …) are APRS protocol: never translated, never split
  across messages, and identical in both applications because they go on the air.
- **Schema version marker.** The classic BBS writes `schema.version` into
  `db_settings` once it has migrated, and its sessions refuse to start until the
  marker is present, because the migration is a one-shot rather than per
  connection. The web app does not read the marker, but it must not delete the row
  or write over it with a different value.
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

**Acknowledgements are entirely the BBS's job, and that is deliberate.** The
receiver acknowledges every inbound message addressed to an APRS-enabled account,
whether the recipient ever opens the web app, is asleep, or has never logged in
there at all — the acknowledgement is owed to the sender, not to a session. The web
app therefore has no ACK path to duplicate, fall out of step, or fire twice, and a
user who never uses the terminal is still acknowledged.

The acknowledgement is sent **from the SSID the message was addressed to**: a
message to `EA7KLK-15` is acknowledged by `EA7KLK-15`, because that is the station
the sender will compare against. All *outgoing* messages still leave from SSID 0 —
see `aprsSSID0` — which is why both surfaces warn about it before sending.

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
