# ![fun text logo](res/scriptorium.png)

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

- Multipart upload through `POST /file/upload` (synchronous) or `POST /file/imports/upload` (background batch).
- Maximum upload size: 100 MiB per file; background batches are limited to 1,000 files and 1 GiB total; at most 10 jobs may be queued or running at once.
- Files are not rejected solely because their extension is unfamiliar.
- Content type is identified from the file signature where possible, with extension-based MIME fallback.
- Unknown formats are stored as `application/octet-stream` and remain downloadable.
- Every upload receives a library record, even when the user supplies no metadata.
- Original filename, extension, MIME type, byte size and SHA-256 digest are recorded.
- Original files are preserved in storage under generated identifiers.
- Imports are staged to a private temporary file before processing.
- SHA-256 duplicate detection avoids storing the same upload twice; concurrent duplicates are protected by a transactional content-hash registry.
- Plain text, Markdown, CSV, logs and reStructuredText have bounded UTF-8 text extraction (up to 1 MiB per item).
- PDFs have optional bounded text extraction using Poppler's `pdftotext`. Extracted text participates in full-text search and suggested Dewey classification. PDF signatures are recognized even with a missing or misleading filename extension.
- Document details explain extraction outcomes: complete, truncated, failed, unsupported, unavailable tool, or no extractable PDF text.
- Existing items can be re-extracted from their stored originals using **Re-extract text** (requires `edit` permission). This refreshes text/search and unconfirmed Dewey suggestions without replacing originals or user-edited metadata.
- Catalogue writes roll back the stored original if persistence fails.
- Background batch jobs are persisted in SQLite and recover after process restarts. Committed per-file outcomes are retained; files whose outcome was not committed may be replayed safely because content hashes prevent duplicate catalogue entries.
- The **Imports** screen lists the latest 25 jobs for the shared library, automatically updates active jobs, and shows per-file imported/duplicate/failed outcomes. Successful and duplicate files link to their catalogue records.
- Batch uploads link directly to their import details. Any authenticated user can inspect import history, including read-only accounts. File-result details are available for newly processed jobs; older completed jobs retain their existing aggregate progress only.
- Directory imports are available only below administrator-configured `IMPORT_ROOTS`; the API accepts a root ID and relative path, never an arbitrary server filesystem path. Symlink entries inside imported trees are skipped.
- Batch source files are staged below `STORAGE_PATH/.jobs` with restrictive permissions before the job is committed to the queue.

### In-app reader — work in progress

Use **Open** in document details without leaving Scriptorium. Reading and saving
a personal place are available to every authenticated account, including read-only
accounts; neither requires upload or catalogue-edit permission.

**The full-format reader milestone is not finished.** Current coverage is:

| Format | Current reading experience |
| --- | --- |
| PDF | Complete document with paging, page jumps, zoom and a selectable-text panel; resumes page, zoom and within-page scroll |
| UTF-8 text | Complete text, paginated into selectable sections without dropping characters; resumes section and scroll |
| Markdown, HTML, CSV, XML, reStructuredText, logs | Complete inert source text where detected as UTF-8; formatted/structured readers still outstanding |
| Signature-recognized raster images | Native image view |
| Signature-recognized audio/video | Native controls with saved playback time; codecs depend on the browser/WebView |
| EPUB, Word, Excel, PowerPoint, OpenDocument and RTF | Proper renderers still outstanding; download-only is not counted as reader support |

PDF.js, its worker, fonts and decoders are bundled locally. No Poppler, Pandoc or
CDN is required to read a PDF. PDF scripts, XFA, interactive forms and links are
not activated; password entry is not implemented. Embedded images are not
intentionally omitted, although large pages/decoded images may be scaled. The
optional selectable-text panel has a resource limit and reports a failure rather
than presenting a truncated prefix as complete; it does not truncate the PDF.

The old **View as PDF** action is replaced by **Open**. The explicit Pandoc API
remains experimental and is not automatically invoked by the current reader.
Reader inputs share the 100 MiB ingestion limit. The API supports HEAD and byte
ranges, but this initial client buffers the original before displaying it; text
pagination limits DOM/layout work, not network transfer. It is not a streaming
media player. PDF parsing and page rendering have best-effort 30-second deadlines.
Input/canvas limits are not hard decoder-memory or CPU bounds; see `SECURITY.md`.

Saved places live in SQLite under a stable account ID and document UUID. They
survive server/browser restarts and ordinary metadata edits. New accounts never
inherit places from a deleted account with the same username. Saves are serial,
checkpoint continuous activity approximately once per second, and flush on close
or page hide. Optimistic versions prevent a stale window from silently replacing
a newer place. Errors remain visible; **Save my current place** is an explicit
choice to retry using this window's position. Network/forced-process failures can
still lose activity since the last acknowledged checkpoint; there is no offline
pending-save queue yet. Legacy originals without hashes use their stored identity
fields, not a newly calculated content digest, to bind reading state.

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

1. Automatic watch-folder monitoring, import cancellation/retry controls, and a configured-directory import UI.
2. PDF metadata extraction, OCR, bulk/background reprocessing, and metadata/text extraction for EPUB, Office and other binary document formats.
3. Full Dewey schedules and a richer classification workflow, including review queues and user overrides in the UI.
4. EPUB/Office readers, formatted Markdown, streaming media, caption tracks, PDF annotations/password entry, and offline pending-place synchronization.
5. Multi-user accounts, roles, sharing permissions and stronger account recovery.
6. Backup/restore, structured audit logging, monitoring and isolated conversion workers.
7. CI, dependency vulnerability gates, reproducible builds, integration tests and release packaging.

## Prerequisites

- Go 1.23 or later.
- CGO enabled and a working C compiler (GCC or Clang; Xcode Command Line Tools on macOS).
- Node.js 22.13 or later (a supported LTS release is recommended) and npm.
- A modern ES2022-capable browser/WebView with module workers for the file viewer.
- Poppler's `pdftotext` is optional and needed for PDF text extraction (installation below).
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
| `STORAGE_PATH` | `./storage` | Root directory for original files, derivatives and private ingestion-job staging |
| `IMPORT_ROOTS` | unset | Optional platform-separated list of existing server-side directories that authenticated users may import from; never accepts arbitrary client paths |
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

## Optional PDF text extraction

Install Poppler **on the backend host** and make `pdftotext` available on the
service's `PATH`. It is not bundled in release archives.

```bash
# Debian / Ubuntu
sudo apt-get install poppler-utils
# Fedora
sudo dnf install poppler-utils
# macOS (Homebrew)
brew install poppler
```

Newly imported PDFs are indexed automatically. Installing Poppler does not
reprocess existing records automatically, and uploading identical content returns
the existing record without re-extraction. To refresh an existing item, open its
details and select **Re-extract text** (requires `edit` permission).

| Extraction status | Meaning |
|---|---|
| `complete` | Text was extracted and indexed |
| `truncated` | Only a bounded UTF-8 prefix was indexed |
| `unavailable` | `pdftotext` was not found; the original remains downloadable |
| `no_text` | The PDF had no extractable text; it may be a scan requiring OCR |
| `failed` | Extraction failed or timed out; the original is retained |
| `unsupported` | No extraction adapter handles this format |

PDF extraction has a 100 MiB input limit, a 1 MiB captured-text limit, a 30-second
timeout including staging and queueing, and at most two concurrent subprocesses.
Temporary copies are private and removed after extraction. Output-limit
truncation stops the subprocess without splitting a UTF-8 character. Corrupt,
password-protected, or image-only PDFs remain accepted as library files; text
extraction is not guaranteed. No OCR or embedded PDF metadata extraction is
performed.

These bounds are **not an OS sandbox or a subprocess memory limit**. Keep Poppler
patched and isolate the service with host/container resource limits when handling
untrusted PDFs; see `SECURITY.md`. CI installs Poppler to exercise real extraction
and full-text indexing tests. Those integration tests skip locally if the optional
tool is absent; deterministic process/failure tests still run.

### Re-extracting existing items

`POST /file/reextract/:uuid` re-extracts one existing item synchronously and returns
its updated public `metadata`. It accepts no editable metadata or filesystem-path
parameters. This works for every supported extraction adapter, not just PDF.

- Stored originals are read, not replaced. If a SHA-256 digest exists, it must
  match before extraction. Legacy items without a digest can still be processed;
  this operation does not backfill or change their ingestion identity.
- Titles, authors, dates, document types, subjects, extra metadata, and confirmed
  Dewey classifications are preserved. Only extraction/index fields, unconfirmed
  suggestions, and the update timestamp change.
- Concurrent metadata edits or deletion cause a conflict rather than being
  overwritten. Saving an older metadata editor also preserves newer extraction
  fields.
- Failed, unavailable, unsupported, or cancelled attempts retain previous indexed
  text **and its extraction status**. A successful `no_text` result intentionally
  removes old extracted text. A truncated success replaces it with a bounded prefix.
- At most two reprocessing requests run at once, including source staging. The
  same item cannot be processed twice concurrently. Each attempt has a one-minute
  outer deadline and a 100 MiB source limit, in addition to the PDF adapter limits.
- Closing document details or navigating away aborts the browser request. This is
  best-effort cancellation, not an undo of a transaction that already committed.
- There is no durable reprocessing queue, bulk action, or automatic retry yet.

The endpoint requires `edit`, not merely `upload`. Errors include `400` for an
invalid UUID, `404` for a missing record, `409` for concurrent changes, an active
attempt on the same item, or an unavailable/corrupt original, `422` for unsupported
or failed extraction, `429` when both reprocessing slots are busy (`Retry-After: 2`),
and `503` for a missing optional extractor or unavailable catalogue capability.
An outer deadline returns `504`; cancellation returns `408` if the connection is
still available to receive a response.

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

### Frontend tests

```bash
cd src/frontend/frontend
npm ci
npm test
npm run check
npm run build
```

`predev` and `prebuild` copy the installed PDF.js fonts, CMaps and decoder assets
into the generated `public/pdfjs/` directory; do not edit that directory. The PDF
engine is lazy-loaded only when opening a PDF. Wails and browser distributions
include the same local viewer assets.

Vitest runs query/pagination unit tests and Svelte component tests in jsdom.
Search tests cover field filters across pages, stale responses, debounce and
navigation cleanup, retrying failed pages, and loading/error/empty states.
CI runs these tests alongside the backend tests and frontend checks.

Run the production-bundle viewer smoke tests with Chromium:

```bash
cd src/frontend/frontend
npx playwright install chromium # use --with-deps on a fresh Linux CI host
npm run test:browser
```

These tests use mocked API responses with real browser rendering: two-page PDF
navigation, local font loading, inert PDF actions/markup, worker teardown,
image/audio decoding, codec fallback, focus restoration and original downloads.
They do not replace backend authorization tests or native Wails platform testing.

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
| `POST` | `/file/upload` | Upload and catalogue one file synchronously (100 MiB maximum) |
| `POST` | `/file/imports/upload` | Queue multipart files (`files` field; `file` also accepted) for background import (1,000 files / 1 GiB per batch) |
| `GET` | `/file/imports/jobs` | List the latest 25 import-job summaries (without per-file results) |
| `GET` | `/file/imports/jobs/:id` | Retrieve one import job, including its persisted file results |
| `GET` | `/file/imports/roots` | List configured import-root IDs and display names |
| `POST` | `/file/imports/directory` | Queue a directory/file import using `{ "root_id": "…", "path": "relative/path" }` |
| `GET` | `/file/download/:uuid` | Download the original file |
| `GET`, `HEAD` | `/file/preview/:uuid` | Authenticated, signature-checked inline preview; supports byte ranges, rejects unsupported formats and inputs over 100 MiB |
| `GET`, `PUT` | `/file/reading-state/:uuid` | Load/save the session account's private place; PUT requires source revision and expected version, but no catalogue-edit permission |
| `POST` | `/file/convert/:uuid` | Experimental explicit Pandoc conversion; not used by the reader |
| `POST` | `/file/reextract/:uuid` | Re-extract one stored original and refresh indexed text (`edit` permission) |

Job details include a `results` array when per-file outcomes are available. Each
entry contains `filename`, `status` (`imported`, `duplicate`, or `failed`), and
optionally `document_id` or a public `error`. Results are in source order and
cover processed files only; staged server paths are never included. A duplicate
links to the existing catalogue record, rather than creating another record.
The UI polls while jobs are active; use **Refresh imports** to discover newly
submitted jobs once all displayed jobs have finished. Failed files must currently
be uploaded again; retry/cancel job actions are not implemented.

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

# Queue a multi-file upload and poll the returned job ID.
curl -X POST http://127.0.0.1:8080/file/imports/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F 'files=@./book.pdf' -F 'files=@./notes.md'

# For configured server-side roots, list IDs then queue a relative directory.
curl -fsS http://127.0.0.1:8080/file/imports/roots -H "Authorization: Bearer $TOKEN"
curl -X POST http://127.0.0.1:8080/file/imports/directory \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"root_id":"ROOT_ID_FROM_PREVIOUS_CALL","path":"research"}'

curl -X POST http://127.0.0.1:8080/data/classify \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Introduction to Go programming","content":"Software architecture and databases"}'
```

Browser clients use HttpOnly cookies plus CSRF headers; the frontend handles
this automatically. Bearer tokens are intended for trusted clients and should
be kept in memory rather than persisted in browser storage.

### multi-user auth and permissions

The backend supports multiple accounts against one shared Scriptorium catalogue.

## Permissions

- `upload`: `POST /file/upload`, `POST /file/imports/upload`, `POST /file/imports/directory`, `POST /file/convert/:uuid`, and `POST /data/create`.
- `edit`: `PUT /data/update` and `POST /file/reextract/:uuid`.
- `delete`: `DELETE /data/delete`.
- Any authenticated account can browse, search, read, download, inspect import jobs, and use classification suggestions.
- The bootstrap owner is an administrator and always retains all three permissions. Only an administrator can manage accounts.

New accounts default to no write permissions unless permissions are supplied in the create request. This is a shared library; permissions are not per-document ownership rules.

## Authentication response

`POST /auth/login` and `GET /auth/me` now include a `user` object with `username`, `admin`, `disabled`, and `permissions` (`upload`, `edit`, `delete`). The login response still includes `session_token` and `csrf_token`, preserving the existing authentication contract.

## Administrator endpoints

All endpoints below require an authenticated administrator. Mutating requests must include the existing CSRF token when using cookie authentication; bearer-token requests retain the existing CSRF exemption.

- `GET /auth/users` — returns `{ "users": [...] }`.
- `POST /auth/users` — create an account with `username`, `password`, and `permissions`:

```json
{"username":"reader","password":"a-long-unique-password","permissions":{"upload":false,"edit":false,"delete":false}}
```

- `PUT /auth/users/:username` — update `permissions` and `disabled`:

```json
{"permissions":{"upload":true,"edit":false,"delete":false},"disabled":false}
```

Disabling an account immediately revokes its active sessions. Permission changes take effect on the next request. The administrator account cannot be disabled or have its full permissions removed.

- `DELETE /auth/users/:username` — permanently remove an account. The last administrator cannot be deleted.

## Storage and migration

The existing `AUTH_FILE` remains the source of truth (default `./scriptorium-auth.json`). A legacy single-account file is migrated automatically: its existing account becomes the administrator, with all permissions enabled. Subsequent changes are persisted atomically in the same file with restrictive file permissions. The library database and stored documents are not modified by this migration.

## Testing

```bash
cd src/backend
CGO_ENABLED=1 go test -tags sqlite_fts5 ./...
```

Tests use temporary directories and databases. The suite includes storage traversal/symlink protections, format identification and extraction, classification suggestions, SQLite CRUD and FTS indexing, BoltDB migration, duplicate import handling, and upload/catalogue metadata preservation. Run the full suite after dependencies are available; a build with CGO disabled is not supported by the SQLite driver.

## Project status

The current implementation remains an early foundation, not a finished universal reader. Unknown files can be stored and catalogued; a bounded set of text formats can be extracted and indexed; SQLite is the primary catalogue store; and existing BoltDB catalogues are migrated with a retained backup. Watch-folder monitoring, broader format extraction, embedded rendering, multi-user access control, backup/restore, and isolated conversion workers remain outstanding. Authentication is currently implemented for a single owner account; see `SECURITY.md` for its limits.


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
