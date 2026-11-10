# Ham BBS Web

A Go/GORM web front end for `ea7klk/simple-ham-bbs`. It reads and writes the
same SQLite database as the classic SSH BBS, so an operator can offer both
surfaces over one set of accounts, boards, bulletins, and APRS history.

## What is implemented

- Compatible GORM models for users, bulletins, boards, threaded messages, and APRS history.
- Login and registration using the same `pbkdf2_sha256$iterations$salt$digest` password format and iteration count (200,000).
- Profile editing, station directory, bulletins, message boards, sysop user administration, and APRS send/history screens.
- APRS receive processing and APRS ACK handling stay in the classic SSH BBS container; this image does not run an APRS receiver or ACK worker.
- SQLite opened at `BBS_DB_FILE` with WAL, a busy timeout, and immediate transactions, matching the original app.
- Dockerfile and Dockge-friendly compose stack, plus a GitOps/Fleet deployment path.

## Compatibility with the SSH BBS

Both applications write to one file, so the safety rules that the SSH BBS gained
in its security review are enforced here too, against the same rows:

- **Reply nesting is capped at five levels** (`maxReplyDepth`), the same cap as
  `simple-ham-bbs`. A reply beyond it is refused with the translated
  `thread_too_deep` message and no row is written.
- **User text is sanitised on write and again on read** with the same rule as
  `bbs/cmd/bbs/sanitize.go`: ESC, C0/C1 controls, DEL, and the invisible
  bidi/zero-width characters are removed, line breaks normalised, and blank-line
  runs capped. A row written by an older client is cleaned when it is displayed.
- **`BBS_SYSOPS` is a bootstrap grant only.** It promotes an account only while
  the database has no enabled sysop; afterwards the configured list has no
  authority and a configured callsign can be demoted like any other.
- **Writes are row-level and transactional**, with the database opened
  `?_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate`, so a web write and
  an SSH write cannot interleave into a lost update.
- **Login attempts are throttled**: five failures for one account from one source
  address within fifteen minutes lock that pair out for fifteen minutes. The
  counter is in process memory (the web app is one process); the SSH app keeps
  its equivalent in SQLite because every SSH session is its own process.

The database file also holds the `db_auth_failures` table owned by the SSH BBS.
The web app neither creates nor reads it.

## Run locally

```sh
cp .env.example .env
mkdir -p data/bbs
BBS_DATA_DIR="$PWD/data/bbs" \
BBS_DB_FILE="$PWD/data/bbs/bbs.sqlite" \
go run ./cmd/ham-bbs-web
```

Open `http://localhost:8080`.

## Docker / Dockge

Use `compose.dockge.yaml` for the full stack. It runs:

- `hamnet`, the WireGuard network namespace and published ports.
- `bbs`, the original SSH BBS image.
- `bbs-web`, this web application.

Both `bbs` and `bbs-web` mount `./data/bbs` to `/var/lib/bbs`, and both use:

```text
BBS_DB_FILE=/var/lib/bbs/bbs.sqlite
```

More detail is in [docs/shared-database.md](docs/shared-database.md).

The web app listens on container port `8080` and is exposed only to the Docker
network. In Dockge, attach your Traefik routing to the `bbs-web` service.

## Published image and deployment

The GitHub Actions workflow in `.github/workflows/docker-image.yml`:

- runs the Go tests with coverage on every branch, pull request, release, and
  manual dispatch;
- builds the image on every run, so a broken Dockerfile fails before a release;
- publishes `linux/amd64` images to `ghcr.io/<owner>/<repo>` on version tags and
  published GitHub releases (`v1.2.3`, `1.2`, `sha-…`, and `latest`);
- pins the published digest in the GitOps repository when `INFRA_REPO_TOKEN` is
  configured.

Production deployment is Fleet, not `docker compose` on the host: the k3s
cluster runs a `bbs-web` Deployment pinned to an image digest, and Fleet
reconciles it with `correctDrift` enabled. The only durable way to ship a release
is to bump that digest in `spainip/infra` → `fleet/managed-apps.yaml`. A manual
`docker compose up` or `kubectl set image` is reverted by Fleet. Without the
`INFRA_REPO_TOKEN` secret the workflow prints the digest in the job summary and
the pin is made by hand.

Set `BBS_WEB_IMAGE` in `.env` if your GHCR package name differs from the default
`ghcr.io/ea7klk/ham-bbs-web`.
