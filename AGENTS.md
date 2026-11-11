# Project Agent Instructions

Guidance for AI agents (Codewhale, Claude Code, etc.) working in this repository.

## What this is

`ham-bbs-web` is a Go/GORM web front end for the classic SSH BBS in
`ea7klk/simple-ham-bbs`. Both applications open the **same SQLite file**, so this
repository is not independent: its data model and its write rules have to keep
matching the SSH BBS. Read `docs/shared-database.md` before changing anything
that touches the database.

## Build, test, lint

```sh
go build ./...        # build every package
go test ./...         # full test suite (SQLite temp files per test)
go test -run TestReplyDepthIsCappedAtMaxDepth -v ./...
go vet ./...          # vet, run before every commit
gofmt -l .            # must print nothing; use gofmt -w . to fix
```

Tests create their own SQLite file in `t.TempDir()` and never touch a real
BBS database. `newTestServer` (`test_helpers_test.go`) is the standard fixture;
it waits for background work before the temp directory is removed, so any new
fire-and-forget goroutine started from a handler must go through
`s.background`.

Docker build (no Docker daemon is required on the deployment host, but a local
daemon is useful for a smoke test):

```sh
docker build -t ham-bbs-web:test .
docker run --rm -p 8080:8080 -v "$PWD/data/bbs:/var/lib/bbs" ham-bbs-web:test
```

## Architecture

Single binary, one `main` package in `cmd/ham-bbs-web`. There is no framework and
no ORM layer beyond GORM; the pieces are:

- `main.go` — starts the server.
- `config.go` — environment parsing, `server` struct, sysop bootstrap helpers,
  `maxReplyDepth`, translations loader. `server` holds the DB handle, parsed
  templates, in-memory sessions, and the login throttle.
- `database.go` — opens SQLite, sets the DSN, and runs `AutoMigrate` over the
  shared models. Every shared model lives in `models.go` and must match the SSH
  BBS exactly.
- `router.go` — `net/http` `ServeMux` routes, auth/sysop middleware, session
  cookie handling, security headers.
- `handlers_*.go` — one file per area: auth/profile, boards/messages, bulletins,
  APRS, admin. Handlers take the authenticated `*dbUser` where the route
  requires one.
- `threads.go` — message creation and editing: sanitising, depth cap, board
  materialisation, sibling position, all inside one immediate transaction.
- `accounts.go` — activation codes, link tokens, the email-change pair, the
  per-address registration meter, and the daily cleanup of those tables.
- `handlers_account.go` — the public pages that redeem them (`/activate`,
  `/forgot`, `/reset`, `/email/confirm`) plus `/about`, `/resources` and the
  sysop message editor.
- `mail.go`, `mail_content.go` — SMTP configuration and the four-language message
  bodies. Same environment variables as the SSH BBS, so one SMTP setup serves
  both.
- `sanitize.go` — the text rule shared with the SSH BBS. A change here is a
  change to the shared contract; port it to `simple-ham-bbs` too.
- `ratelimit.go` — in-memory login throttle (five failures per account and
  source address per fifteen minutes, fifteen-minute lockout).
- `security.go` — PBKDF2 password hashing/verification and session tokens.
- `templates.go` — all HTML templates, the stylesheet, and the template funcs,
  embedded in the binary. The look is "Packet Manual structure, Spectrum
  palette": monospace throughout, hard rules, uppercase letter-spaced labels,
  ruled tables and a title block, on a deep indigo background lit by cyan,
  violet, magenta and lime. Keep new markup on that grid rather than
  reintroducing rounded cards: `.card`/`.panel` for a box, `.strip` for a section
  header, `.rows`/`.rowlink` for a list, `.status`/`.tiles` for metrics, and
  `.pagehead` for the heading of a page. Inline `style=` attributes are not used;
  spacing utilities (`.mt`, `.mt-lg`, `.mb`, `.wide`, `.narrow`) cover the rest.
  Every colour pair in the palette clears WCAG AA for body text, and the
  structural rule `--rule-hi` clears 3:1 against the page background — keep both
  when changing a colour.
- `translations.json` — UI strings for `en`, `es`, `fr`, `de`; loaded at runtime
  (path in `BBS_TRANSLATIONS_FILE`) and also copied into the image. Adding a
  user-visible string means adding a key to **all four** languages.
- `handlers_aprs.go` — APRS send and history views. It sends; it does not
  receive, encode/decode packets for the air, or run an ACK worker.

Data flow: request → `routes()` → optional `requireAuth`/`requireSysop` →
handler → GORM against the shared database → `s.view` renders a template with
`viewData`. Sessions are an in-memory token → callsign map; a restart logs
everyone out (acceptable for a single-replica deployment).

## Invariants that must not regress

1. **Shared schema.** `dbUser`, `dbBulletin`, `dbBoard`, `dbMessage`,
   `dbAPRSSent`, `dbAPRSSentPart`, `dbAPRSReceived` are byte-for-byte the same
   structs as `simple-ham-bbs/bbs/cmd/bbs/database.go`. Do not rename fields,
   add columns, or change tags here alone.
   `dbUser.Email` carries a `COLLATE NOCASE` index, and every lookup by address
   must name the collation or it scans: see `emailLookup` in `accounts.go`.
   `dbUser.Registered` is write-once: set it at creation, never on update. Both
   applications backfill it from `first_seen` on startup. A handler that builds
   a fresh `dbUser` from a form must carry `Registered` and `FirstSeen` over
   from the loaded row before saving, or the Save blanks them.
2. **DSN.** `?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate`. The
   `_txlock=immediate` part is the cross-process guarantee: read-then-write
   transactions take the write lock at `BEGIN`.
3. **Row-level writes.** Never `DELETE` a table and re-insert a snapshot. Every
   mutation names the rows it changes and runs in one transaction.
4. **Reply depth.** `maxReplyDepth = 5` (a root plus five nested replies). It is
   enforced in `createMessage`, in the storage layer, not only in the UI. The
   board page hides the reply form at the limit.
5. **Sanitising.** Every user-supplied string that is stored or rendered goes
   through `sanitizeSingleLineText` or `sanitizeMultilineText`, on write. Read
   paths for content that may predate the rule clean it again. Passwords are
   never sanitised.
6. **Sysop bootstrap.** `isSysop` answers from the stored row. `BBS_SYSOPS`
   grants the role only while no enabled sysop exists (`isBootstrapSysop`,
   `applyConfiguredSysops`), and that check fails closed.
7. **Last sysop.** A sysop cannot change their own role or disable themselves
   through `/admin/users`; because every caller of that route is an active
   sysop, at least one enabled sysop remains.
8. **Auth throttle.** Login failures are counted per account and peer address;
   a success clears the counter. Do not key it on the peer address alone — the
   app sits behind a reverse proxy, and one shared bucket would let any caller
   lock out the whole site.
9. **A page renders to a buffer, not straight to the response.** `viewStatus`
   executes the template into memory first, then sets the status. html/template
   writes as it goes and only reports an error at the end, so streaming straight
   into the response would emit a truncated page with a 200 — and a 404 could
   never carry a body. Missing content goes through `s.notFound`, which renders
   the real page with the caller's own navigation.
10. **Token formats are a wire format between the two applications.** A reset
   link the SSH BBS mails must be redeemable by `claimToken` here, and a code
   issued here must be verifiable there. Changing the hash input, the purpose
   strings, the alphabet or a TTL is a change to both repositories, and it
   invalidates any link already in someone's inbox.
11. **Activation is checked after the password.** An unactivated account gets no
    session, but it is not told the account exists before the password checks
    out either. Registration does not activate: with SMTP configured it mails a
    code, and without SMTP it marks the account active rather than locking the
    caller out of an unconfigured deployment.

## Configuration

Environment variables, with defaults in `config.go` and `.env.example`:

- `BBS_WEB_ADDR` (`:8080`), `BBS_DATA_DIR` (`/var/lib/bbs`),
  `BBS_DB_FILE` (`$BBS_DATA_DIR/bbs.sqlite`), `BBS_TRANSLATIONS_FILE`.
- `BBS_NAME`, `BBS_SYSOP`, `BBS_SYSOPS`, `BBS_LOCATION`, `BBS_WELCOME_TOPIC`.
- `APRS_IS_SERVER`, `APRS_IS_PORT` for outbound APRS sends.
- `BBS_SMTP_HOST`, `BBS_SMTP_PORT`, `BBS_SMTP_SECURITY`, `BBS_SMTP_USERNAME`,
  `BBS_SMTP_PASSWORD`, `BBS_SMTP_FROM`, `BBS_SMTP_FROM_NAME`, `BBS_WEB_BASE_URL`
  for outbound mail and for the links inside it. These are the SSH BBS's own
  variables; leaving them unset disables mail and skips activation.

  **Configure SMTP on both surfaces or on neither.** With mail, registration is
  not activated until the emailed code is entered. Without mail, both
  applications activate immediately because there is no way to send a code.
  Setting `BBS_SMTP_HOST` only on the SSH container therefore leaves the web
  form as an unverified registration path that the terminal will not accept —
  and the reverse leaves the terminal as a way around a web-only policy. The
  deployment wires the same `bbs-smtp` secret into both containers for this
  reason; keep it that way.
- `BBS_GUEST_MODE`, `BBS_USER_PASSWORD` are consumed by the SSH BBS image, not
  by this one; they are in `.env.example` because the compose stack is shared.

## GitHub operations

Use the `gh` CLI for **all** GitHub operations — issues, pull requests, reviews,
releases, workflow runs, repository settings, and API queries. It is already
installed and authenticated in this environment, so do not ask for a token or
re-run `gh auth login`.

- Prefer the specific subcommand (`gh release`, `gh run`, `gh pr`, `gh issue`,
  `gh api` only for what has no subcommand).
- Confirm the repository with `gh repo view ea7klk/ham-bbs-web` before mutating
  anything, and never push, publish, or change settings unless the user asked.
- Treat `gh` output as ground truth; read it rather than assuming success.

## Release and deployment

Releases are tags/releases in `main`. Publishing a GitHub release triggers
`.github/workflows/docker-image.yml`, which runs the tests, builds the image,
pushes `ghcr.io/ea7klk/ham-bbs-web` (version, major.minor, sha, latest), and
tries to pin the digest in `spainip/infra` when `INFRA_REPO_TOKEN` is set.

The production cluster is k3s, reconciled by Fleet with `correctDrift: true`,
running `bbs` and `bbs-web` Deployments pinned to image digests.

**To deploy:**

1. Tag and publish the release (or run the workflow manually with the version).
2. Wait for the `Docker image` workflow to succeed and read the digest from the
   job summary.
3. In `spainip/infra`, update the `ghcr.io/ea7klk/ham-bbs-web@sha256:…` reference
   in `fleet/managed-apps.yaml`, commit, and push.
4. Confirm Fleet reconciled: `kubectl -n fleet-local get gitrepo infra` shows the
   new commit and `kubectl -n ham-bbs rollout status deploy/bbs-web`.

Never deploy with `docker compose up` or `kubectl set image` on the cluster host:
there is no Docker daemon there and Fleet reverts drift.

## Commit messages

Conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`,
`ci:`. One logical change per commit; say why in the body when the reason is not
obvious from the diff. A compatibility change should name the SSH BBS change it
mirrors (for example "mirror the M8 reply-depth cap").
