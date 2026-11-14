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
│       ├── formats/         # Content-type identification
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

### Storage safety

- Storage operations reject absolute paths and parent-directory traversal.
- Existing symlink components are rejected to prevent storage-root escapes.
- Files are created exclusively with restrictive permissions.
- Failed writes remove partial files.

### Dewey classification

- Existing Dewey category endpoint: `GET /data/dewey`.
- `POST /data/classify` accepts title, author and/or text and returns ranked suggestions with evidence and a heuristic confidence score.
- Uploads may receive a suggested class based on their title and author.
- Suggestions remain suggestions; setting a Dewey value through the metadata editor marks the classification as confirmed.
- The current keyword rules are deliberately conservative and are not a complete DDC classifier or a replacement for the full DDC schedules.

### API

- `GET /healthz` reports that the HTTP server is responding.
- REST API served on the configured host and port.
- When `WEB_ROOT` contains a built Svelte application, the backend serves its static assets and falls back to `index.html` for browser-side routes.
- File transfer uses the storage abstraction directly; the unauthenticated internal gRPC listener has been removed from application startup.
- Metadata updates preserve ingestion fields such as the storage path, digest, MIME type and file size.

## Not implemented yet

These are explicit remaining product milestones, not implied capabilities:

1. Replace BoltDB with SQLite and introduce versioned schema migrations.
2. Add background ingestion jobs and format-specific metadata/text extraction adapters.
3. Add robust duplicate detection and safe re-import workflows.
4. Add full Dewey hierarchy and a richer classification workflow, including manual review queues.
5. Add full-text indexing and search within document contents.
6. Implement embedded PDF, EPUB, text, image, audio and video readers.
7. Add authentication, user/session management and LAN access controls before treating the service as safe for untrusted networks.
8. Add backup/restore, health checks, structured logging and graceful HTTP server shutdown.
9. Add CI, reproducible builds, integration tests and release packaging.

## Prerequisites

- Go 1.23 or later.
- Node.js 18 or later and npm.
- Pandoc is optional and only needed for document conversions that Pandoc supports.
- Wails v2 CLI for the desktop build.

## Backend setup

```bash
cd src/backend
cp ../../.env.example .env # optional; export the values in your shell or use your preferred dotenv loader

go test ./...
go run .
```

The backend reads environment variables directly; it does not currently load `.env` files automatically.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DB_PATH` | `./scriptorium.db` | BoltDB metadata database path (legacy persistence; migration to SQLite is planned) |
| `DB_MODE` | `0600` | Database file permissions, in octal |
| `STORAGE_PATH` | `./storage` | Root directory for original files and derivatives |
| `REST_HOST` | `127.0.0.1` | API bind address; use `0.0.0.0` for LAN access |
| `REST_PORT` | `8080` | API port |
| `WEB_ROOT` | `./web` | Directory containing the built browser frontend |
| `VITE_API_BASE_URL` | `http://localhost:8080` | API base URL used by the Svelte frontend |

**LAN security warning:** the current API does not authenticate users. Bind to loopback unless you are on a trusted private network and understand that anyone who can reach the API can read, upload, modify and delete library records. Do not expose it directly to the internet. Authentication and access control are release blockers for broader deployment.

For LAN access, set `REST_HOST=0.0.0.0`, allow the chosen TCP port through the host firewall, and configure the frontend's `VITE_API_BASE_URL` to the backend's LAN address.

## Frontend development

```bash
cd src/frontend/frontend
npm install
VITE_API_BASE_URL=http://127.0.0.1:8080 npm run dev -- --host 0.0.0.0
```

For a production browser deployment, build the Svelte app and copy its output into the backend's configured web root:

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
| `GET` | `/file/convert/:uuid` | Convert supported document formats with Pandoc |

Example upload:

```bash
curl -X POST http://127.0.0.1:8080/file/upload \
  -F 'file=@./research.customfmt' \
  -F 'metadata={"Title":"Research notes","Author":"Jane Doe"}'
```

Example classification request:

```bash
curl -X POST http://127.0.0.1:8080/data/classify \
  -H 'Content-Type: application/json' \
  -d '{"title":"Introduction to Go programming","content":"Software architecture and databases"}'
```

## Testing

```bash
cd src/backend
go test ./...
```

Tests use temporary directories and databases. The current suite includes storage traversal/symlink protections, format identification, classification suggestions and upload/catalogue metadata preservation.

## Project status

The current implementation is the first foundation pass, not a finished universal reader. Unknown files can be stored and catalogued, but format-specific extraction, content search, embedded rendering, browser production hosting, authentication and the SQLite migration remain outstanding.
