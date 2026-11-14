# Scriptorium

Scriptorium is a local-first digital library intended to accept arbitrary files, catalogue them, organise them using Dewey Decimal Classification (DDC), and make their contents accessible through desktop and browser clients.

The project is under active development. This README distinguishes implemented behaviour from planned capabilities; a file being accepted by Scriptorium does not imply that it can already be rendered or searched internally.

## Product direction

- **Deployment:** self-hosted on a trusted home or office LAN.
- **Clients:** Wails desktop application and a browser client sharing one Go API.
- **Ingestion:** preserve original files, identify content types where possible, and catalogue unknown formats rather than rejecting them.
- **Organisation:** hybrid Dewey classification. The system may suggest a class with evidence and confidence; users confirm or override it.
- **Storage:** local filesystem for original files and derivatives, with a metadata database.

## Repository layout

```text
src/
├── backend/
│   ├── main.go
│   └── internal/backend/
│       ├── classification/  # Explainable Dewey suggestions
│       ├── config/          # Environment configuration
│       ├── converter/       # Pandoc document conversion
│       ├── dao/             # Metadata persistence and Dewey categories
│       ├── fao/             # Storage-root-confined local file access
│       ├── formats/         # Content identification and extraction adapters
│       ├── ingest/          # Staged imports, deduplication and catalogue writes
│       └── service/         # REST API and application services
└── frontend/
    ├── app.go               # Wails integration
    └── frontend/            # Svelte application
```

## Implemented

### File ingestion

- Multipart upload through `POST /file/upload`.
- Maximum upload size: 100 MiB.
- Files are not rejected solely because their extension is unfamiliar.
- Content type is identified from the file signature where possible, with extension-based MIME fallback.
- Unknown formats are stored as `application/octet-stream` and remain downloadable.
- Every upload receives a library record, even when the user supplies no metadata.
- Original filename, extension, MIME type, byte size and SHA-256 digest are recorded.
- Original files are preserved in storage under generated identifiers.
- Imports are staged to a private temporary file before processing.
- SHA-256 duplicate detection avoids storing the same upload twice; concurrent duplicates are protected by a transactional content-hash registry.
- Plain text, Markdown, CSV, logs and reStructuredText have bounded UTF-8 text extraction (up to 1 MiB per item); extraction status distinguishes complete, truncated, failed and unsupported formats.
- Catalogue writes roll back the stored original if persistence fails.

### Storage safety

- Storage operations reject absolute paths and parent-directory traversal.
- Existing symlink components are rejected to prevent storage-root escapes.
- Files are created exclusively with restrictive permissions.
- Failed writes remove partial files.

### Dewey classification

- Existing Dewey category endpoint: `GET /data/dewey`.
- `POST /data/classify` accepts title, author and/or text and returns ranked suggestions with evidence and a heuristic confidence score.
- Uploads may receive a suggested class based on their title, author and extracted text.
- Suggestions remain suggestions; setting a Dewey value through the metadata editor marks the classification as confirmed.
- The current keyword rules are deliberately conservative and are not a complete DDC classifier or a replacement for the full DDC schedules.

### API

- `GET /healthz` reports that the HTTP server is responding.
- REST API served on the configured host and port.
- When `WEB_ROOT` contains a built Svelte application, the backend serves its static assets and falls back to `index.html` for browser-side routes.
- File transfer uses the storage abstraction directly; the unauthenticated internal gRPC listener has been removed from application startup.
- Metadata updates preserve ingestion fields such as the storage path, digest, MIME type and file size.

## Persistence and search

- SQLite is the primary metadata database.
- Schema migrations are recorded in `schema_migrations`.
- Existing BoltDB catalogues are migrated automatically on startup. The original is retained as `<DB_PATH>.bolt.bak`; migration refuses to overwrite an existing backup.
- Extracted text is stored separately from the public metadata JSON and indexed with SQLite FTS5.
- The content-hash registry prevents new duplicate imports while preserving pre-existing duplicate catalogue records during migration.
- SQLite is built using `github.com/mattn/go-sqlite3` with its `sqlite_fts5` build tag, so CGO and a C compiler are required. Use the provided Makefile targets or pass the tag explicitly.

## Not implemented yet

These are explicit remaining product milestones, not implied capabilities:

1. Background ingestion jobs, resumable imports and directory/watch-folder import.
2. Format-specific metadata and text extraction for PDF, EPUB, Office and other binary document formats.
3. Full Dewey schedules and a richer classification workflow, including review queues and user overrides in the UI.
4. Embedded PDF, EPUB, text, image, audio and video readers.
5. Multi-user accounts, roles, sharing permissions and stronger account recovery.
6. Backup/restore, structured audit logging, monitoring and isolated conversion workers.
7. CI, dependency vulnerability gates, reproducible builds, integration tests and release packaging.

## Prerequisites

- Go 1.23 or later.
- CGO enabled and a working C compiler (GCC or Clang; Xcode Command Line Tools on macOS).
- Node.js 18 or later and npm.
- Pandoc is optional and only needed for document conversions that Pandoc supports.
- Wails v2 CLI for the desktop build.

## Backend setup

```bash
cd src/backend
cp ../../.env.example .env # optional; export the values in your shell or use your preferred dotenv loader

CGO_ENABLED=1 go test -tags sqlite_fts5 ./...
CGO_ENABLED=1 go run -tags sqlite_fts5 .
```

The backend reads environment variables directly; it does not currently load `.env` files automatically.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DB_PATH` | `./scriptorium.db` | SQLite metadata database path; an existing BoltDB file at this path is migrated automatically |
| `DB_MODE` | `0600` | Database file permissions, in octal |
| `STORAGE_PATH` | `./storage` | Root directory for original files and derivatives |
| `REST_HOST` | `127.0.0.1` | API bind address; keep private behind a reverse proxy |
| `REST_PORT` | `8080` | API port |
| `WEB_ROOT` | `./web` | Directory containing the built browser frontend |
| `AUTH_FILE` | `./scriptorium-auth.json` | Owner credential file (mode 0600) |
| `AUTH_USERNAME` / `AUTH_PASSWORD` | unset | First-run owner bootstrap only |
| `COOKIE_SECURE` | `true` | Require Secure cookies |
| `ALLOW_INSECURE_COOKIES` | `false` | Explicit local-development escape hatch |
| `CORS_ALLOWED_ORIGINS` | unset | Exact origins for separately hosted clients |
| `TRUSTED_PROXIES` | unset | Exact reverse-proxy IPs/CIDRs for client-IP derivation |
| `VITE_API_BASE_URL` | same-origin in browser; localhost for Wails | API base URL embedded in the frontend build |

**Security warning:** the API now requires an authenticated owner session, but it is not yet ready for public exposure. The application does not terminate TLS itself. For LAN and internet access, use an HTTPS reverse proxy, keep the Go listener private, and follow `SECURITY.md`. Direct HTTP access on a LAN is a development convenience, not the recommended deployment.

## Frontend development

```bash
cd src/frontend/frontend
npm install
VITE_API_BASE_URL=http://127.0.0.1:8080 npm run dev -- --host 0.0.0.0
```

For a production browser deployment, build the Svelte app and copy its output into the backend's configured web root. The browser UI and API should share one HTTPS origin where possible:

```bash
cd src/frontend/frontend
npm install
npm run build
mkdir -p ../../backend/web
cp -a dist/. ../../backend/web/
```

Then run the backend from `src/backend`. Browse to `http://<server-address>:8080`. The API and browser UI will share the same origin. For development, the Vite server remains available separately.

## Desktop development

```bash
cd src/frontend
wails dev
```

Build:

```bash
cd src/frontend
wails build
```

## API endpoints

### Authentication — `/auth`

| Method | Path | Description |
|---|---|---|
| `POST` | `/auth/login` | Authenticate and issue a session |
| `GET` | `/auth/me` | Check the current session and retrieve the CSRF token |
| `POST` | `/auth/logout` | Revoke the current session |
| `POST` | `/auth/password` | Change the owner password and revoke all sessions |

### Library metadata — `/data`

| Method | Path | Description |
|---|---|---|
| `POST` | `/data/create` | Create a metadata record |
| `GET` | `/data/read/:uuid` | Read a metadata record |
| `PUT` | `/data/update` | Update editable metadata without overwriting ingestion fields |
| `DELETE` | `/data/delete` | Delete records and associated stored files |
| `GET` | `/data/search` | Search metadata with pagination |
| `GET` | `/data/types` | List registered document types |
| `GET` | `/data/dewey` | List available Dewey categories |
| `POST` | `/data/classify` | Return explainable Dewey suggestions |

### Files — `/file`

| Method | Path | Description |
|---|---|---|
| `POST` | `/file/upload` | Upload and catalogue any file up to 100 MiB |
| `GET` | `/file/download/:uuid` | Download the original file |
| `POST` | `/file/convert/:uuid` | Convert supported document formats with Pandoc |

All API routes other than `/healthz` and `/auth/login` require authentication.
For scripts and API clients, use the 12-hour bearer token returned by login:

```bash
TOKEN=$(curl -fsS http://127.0.0.1:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"owner","password":"YOUR_PASSWORD"}' | jq -r .session_token)

curl -X POST http://127.0.0.1:8080/file/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F 'file=@./research.customfmt' \
  -F 'metadata={"Title":"Research notes","Author":"Jane Doe"}'

curl -X POST http://127.0.0.1:8080/data/classify \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Introduction to Go programming","content":"Software architecture and databases"}'
```

Browser clients use HttpOnly cookies plus CSRF headers; the frontend handles
this automatically. Bearer tokens are intended for trusted clients and should
be kept in memory rather than persisted in browser storage.

## Testing

```bash
cd src/backend
go test ./...
```

Tests use temporary directories and databases. The suite includes storage traversal/symlink protections, format identification and extraction, classification suggestions, SQLite CRUD and FTS indexing, BoltDB migration, duplicate import handling, and upload/catalogue metadata preservation. Run the full suite after dependencies are available; a build with CGO disabled is not supported by the SQLite driver.

## Project status

The current implementation remains an early foundation, not a finished universal reader. Unknown files can be stored and catalogued; a bounded set of text formats can be extracted and indexed; SQLite is the primary catalogue store; and existing BoltDB catalogues are migrated with a retained backup. Background imports, broader format extraction, embedded rendering, multi-user access control, backup/restore, and isolated conversion workers remain outstanding. Authentication is currently implemented for a single owner account; see `SECURITY.md` for its limits.


## Security and remote access

See [SECURITY.md](SECURITY.md) for the threat model, implemented controls, residual risks, and a reverse-proxy deployment example.

Scriptorium requires an owner account. On first startup, set `AUTH_USERNAME` and
`AUTH_PASSWORD` (at least 12 characters). The credentials are bcrypt-hashed and
stored in `AUTH_FILE`, which is created with owner-only permissions. The
bootstrap environment variables are ignored once that file exists. Back up the
authentication file alongside the database; losing it means the owner account
must be recovered by stopping Scriptorium, preserving a backup, removing the authentication file, and restarting with new bootstrap credentials. This recovery requires filesystem access to the host.

All library and file endpoints require an authenticated session. Sessions are
held in memory, expire after 12 hours, and are invalidated on password change
or server restart. Browser mutations require a CSRF token. Login attempts are
rate-limited per source address. Password changes are available through
`POST /auth/password` with `current_password` and `new_password` fields.

### LAN and internet deployment

The HTTP listener does not provide TLS. For use beyond a trusted local network,
place it behind a maintained reverse proxy such as Caddy, nginx, or Traefik and
terminate TLS there. Expose only the reverse proxy to the internet; keep the
Scriptorium listener on a private interface or private container network. Use
valid HTTPS certificates, restrict inbound firewall access, keep the host and
proxy patched, and configure `COOKIE_SECURE=true` (the default). Do not set it
to false outside local HTTP development. The application refuses that setting
unless `ALLOW_INSECURE_COOKIES=true` is also explicitly set; this escape hatch
should only be used on a trusted development machine.

The browser UI should be served by the same origin as the API. The Wails client
can use a 12-hour in-memory bearer session; browser clients use HttpOnly
session cookies and CSRF protection. Cross-origin frontends require exact entries in `CORS_ALLOWED_ORIGINS`; wildcard origins are
not supported. Do not trust arbitrary forwarded headers. If a reverse proxy is
used, it must strip client-supplied forwarding headers and set its own. The
application trusts no proxy addresses for client-IP derivation unless exact
entries are configured in `TRUSTED_PROXIES`; never trust all addresses.

This release has a single owner account, not multi-user accounts or role-based
access control. It is a security baseline, not a claim of a completed internet
security audit. Do not expose it publicly until TLS proxying, backups, restore,
monitoring, dependency updates, and deployment-specific security checks have
been completed.
