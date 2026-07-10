# Ham BBS Web

A Go/GORM web reimplementation of `ea7klk/simple-ham-bbs` that uses the same SQLite schema and can run beside the original SSH BBS against the same database instance.

## What is implemented

- Compatible GORM models for users, bulletins, boards, threaded messages, and APRS history.
- Login and registration using the same `pbkdf2_sha256$iterations$salt$digest` password format.
- Profile editing, station directory, bulletins, message boards, sysop user administration, and APRS send/history screens.
- APRS receive processing and APRS ACK handling stay in the classic SSH BBS container; this image does not run an APRS receiver or ACK worker.
- SQLite opened at `BBS_DB_FILE` with WAL and a busy timeout, matching the original app.
- Dockerfile and Dockge-friendly compose stack.

## Run locally

```sh
cp .env.example .env
mkdir -p data/bbs
BBS_DATA_DIR="$PWD/data/bbs" \
BBS_DB_FILE="$PWD/data/bbs/bbs.sqlite" \
GOCACHE=/private/tmp/ham-bbs-web-go-build \
GOMODCACHE=/private/tmp/ham-bbs-web-go-mod \
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

More detail is in [docs/shared-database.md](/Users/Volker_Kerkhoff/Documents/Ham-BBS Web/docs/shared-database.md).

The web app listens on container port `8080` and is exposed only to the Docker network. In Dockge, attach your Traefik routing to the `bbs-web` service.

## Published image

The GitHub Actions workflow in `.github/workflows/docker-image.yml` builds multi-architecture images for `linux/amd64` and `linux/arm64` and publishes to:

```text
ghcr.io/<owner>/<repo>
```

Pull requests and manual workflow runs build without publishing. New version tags like `v1.2.3` and published GitHub releases publish to GHCR.

Set `BBS_WEB_IMAGE` in `.env` if your GHCR package name differs from the default `ghcr.io/ea7klk/ham-bbs-web`.
