# Sharing the SQLite database

Both applications must see the same file at the same in-container path:

```text
/var/lib/bbs/bbs.sqlite
```

The Dockge compose file does that by bind-mounting one host directory into both containers:

```yaml
volumes:
  - ./data/bbs:/var/lib/bbs
environment:
  - BBS_DB_FILE=/var/lib/bbs/bbs.sqlite
```

The original SSH BBS and this web app both open SQLite through GORM with:

```text
?_busy_timeout=5000&_journal_mode=WAL
```

That keeps concurrent access practical for this small BBS. Keep the database, `bbs.sqlite-wal`, and `bbs.sqlite-shm` files together when backing up or moving a live system.

## Dockge deployment

1. Copy `.env.example` to `.env` and edit the passwords, sysop callsigns, and APRS callsigns.
2. Put the real WireGuard profile at `hamnet/wg_confs/wg0.conf` if you use the HamNet container.
3. In Dockge, create a stack from `compose.dockge.yaml`.
4. Start the stack.

The SSH BBS is available on `${BBS_SSH_PORT:-2222}`. The web app listens on container port `8080` and is intended to be exposed by your Dockge/Traefik deployment. Both BBS applications use `./data/bbs/bbs.sqlite` on the Docker host.

The classic SSH BBS remains the APRS worker. It receives APRS messages, sends APRS ACKs for received messages, receives ACKs/rejections for sent messages, and updates the shared SQLite tables. The web image only reads the shared APRS tables and can add sent-message rows when a user sends from the web UI; it does not run an APRS receiver or any ACK loop.

## If you already have BBS data

Stop the existing BBS container first, then copy the whole data directory into this stack:

```sh
mkdir -p data/bbs
cp /path/to/current/bbs.sqlite* data/bbs/
```

Start the stack after the copy. Do not copy only `bbs.sqlite` while the old service is running, because WAL mode can keep recent writes in `bbs.sqlite-wal`.
