# Scriptorium

A document management and library system with a Go backend and a Svelte/Wails desktop frontend. Scriptorium stores, indexes, searches, and converts documents of any supported format, using BoltDB for metadata and local filesystem storage for files.

## Architecture

```
src/
├── backend/                  # Go REST + gRPC backend
│   ├── main.go               # Entry point, wiring, graceful shutdown
│   └── internal/backend/
│       ├── config/            # Environment-based configuration
│       ├── converter/         # Pandoc-based file conversion
│       ├── dao/               # Data access (BoltDB), document models, Dewey data
│       ├── fao/               # File access (local filesystem)
│       └── service/           # HTTP handlers, gRPC file streaming, service layer
│           └── pb/            # Protobuf definitions
└── frontend/                 # Wails v2 desktop app
    ├── app.go                # Wails backend (native dialogs)
    └── frontend/             # Svelte SPA
        └── src/
            ├── App.svelte
            ├── config.ts     # API base URL
            └── components/
                ├── Library.svelte    # Browse, search, open, convert, edit, delete
                ├── Add.svelte        # Upload with metadata, Dewey classification
                ├── EditModal.svelte  # Edit document metadata
                ├── ItemCard.svelte   # Grid card for library items
                ├── Settings.svelte   # Preferences (persisted to localStorage)
                └── Sidebar.svelte    # Navigation
```

### How it works

- **REST API** (Gin) handles CRUD, search, and metadata operations on port `8080`.
- **gRPC** streams file uploads and downloads on port `5001`.
- **BoltDB** stores document metadata as JSON in a single `documents` bucket.
- **Local FAO** persists files on disk under a configurable storage directory.
- **Pandoc converter** converts between document formats (e.g. DOCX to PDF).
- **Wails v2** wraps the Svelte frontend into a native desktop application.

## Prerequisites

- **Go** 1.23+
- **Node.js** 18+ and npm
- **Pandoc** (for file conversion) — `sudo apt install pandoc` or equivalent
- **Wails CLI** v2 (for building the desktop app) — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Setup

### 1. Clone and configure

```bash
git clone <repo-url> && cd scriptorium
cp .env.example .env   # edit values as needed
```

### 2. Run the backend

```bash
cd src/backend
go build -o scriptorium .
./scriptorium
```

The backend will start the REST API on `:8080` and gRPC on `:5001` by default.

### 3. Run the frontend (development)

```bash
cd src/frontend
wails dev
```

This launches the Svelte dev server with hot reload inside a native window.

### 4. Build for production

```bash
cd src/frontend
wails build
```

The compiled binary will be in `src/frontend/build/bin/`.

## Configuration

All configuration is via environment variables (or a `.env` file):

| Variable | Default | Description |
|---|---|---|
| `DB_PATH` | `./scriptorium.db` | Path to BoltDB file |
| `DB_MODE` | `0600` | File permissions for the database |
| `STORAGE_PATH` | `./storage` | Directory for uploaded files |
| `REST_PORT` | `8080` | REST API listen port |
| `GRPC_PORT` | `5001` | gRPC listen port |
| `VITE_API_BASE_URL` | `http://localhost:8080` | API URL used by the Svelte frontend |

## API Reference

### Data endpoints — `/data`

| Method | Path | Description |
|---|---|---|
| `POST` | `/data/create` | Create a document record |
| `GET` | `/data/read/:uuid` | Read a document by UUID |
| `PUT` | `/data/update` | Update a document's metadata |
| `DELETE` | `/data/delete` | Bulk delete by UUID list |
| `GET` | `/data/search` | Search with pagination |
| `GET` | `/data/types` | List registered document types |
| `GET` | `/data/dewey` | List Dewey Decimal categories |

#### Search parameters

| Parameter | Description |
|---|---|
| `q` | Fuzzy search across all text fields (title, author, type, Dewey, etc.) |
| `key` | Exact field name to match (e.g. `Title`, `Author`, `DocType`, `DeweyDecimal`) |
| `value` | Value to match against the specified key |
| `page` | Page number (default: 1) |
| `limit` | Results per page, 1–100 (default: 10) |

If `q` is provided it takes priority over `key`/`value`. If neither is provided, all documents are returned.

**Examples:**

```bash
# Fuzzy search
curl "http://localhost:8080/data/search?q=physics&page=1&limit=20"

# Exact field search
curl "http://localhost:8080/data/search?key=Author&value=John%20Doe"

# All documents
curl "http://localhost:8080/data/search"
```

#### Create / Update body

```json
{
  "DocType": "Book",
  "Title": "Introduction to Algorithms",
  "Author": "Cormen et al.",
  "PublishDate": "2009-07-31",
  "DeweyDecimal": "510"
}
```

Update also requires `Uuid` in the body.

#### Delete body

```json
{
  "uuids": ["uuid-1", "uuid-2"]
}
```

### File endpoints — `/file`

| Method | Path | Description |
|---|---|---|
| `POST` | `/file/upload` | Upload a file (multipart form, optional `metadata` JSON field) |
| `GET` | `/file/download/:uuid` | Download a file by document UUID |
| `GET` | `/file/convert/:uuid` | Convert a file and stream the result |

#### Upload example

```bash
curl -X POST http://localhost:8080/file/upload \
  -F "file=@document.docx" \
  -F 'metadata={"DocType":"Book","Title":"My Book","Author":"Jane","DeweyDecimal":"800"}'
```

#### Convert example

```bash
# Convert to PDF (default)
curl "http://localhost:8080/file/convert/<uuid>" -o output.pdf

# Convert to HTML
curl "http://localhost:8080/file/convert/<uuid>?format=html" -o output.html
```

### Supported file types

Documents: PDF, DOCX, DOC, TXT, MD, RTF, ODT, EPUB
Images: JPG, JPEG, PNG, GIF, SVG
Audio: MP3, WAV, FLAC, AAC
Video: MP4, AVI, MOV, MKV

Maximum upload size: **100 MB**

## Frontend search prefixes

In the Library search bar, you can use prefixes for targeted searches:

| Prefix | Example | Behaviour |
|---|---|---|
| *(none)* | `algorithms` | Fuzzy match across all fields |
| `author:` | `author:Knuth` | Exact match on Author field |
| `type:` | `type:Book` | Exact match on DocType |
| `dewey:` | `dewey:510` | Exact match on Dewey Decimal code |
| `filetype:` | `filetype:.pdf` | Exact match on file extension |

## Dewey Decimal Classification

Documents can be categorised using Dewey Decimal codes. The system includes the 10 main classes and their second-level divisions (100 categories total):

| Code | Class |
|---|---|
| 000 | Computer Science, Information & General Works |
| 100 | Philosophy & Psychology |
| 200 | Religion |
| 300 | Social Sciences |
| 400 | Language |
| 500 | Science |
| 600 | Technology |
| 700 | Arts & Recreation |
| 800 | Literature |
| 900 | History & Geography |

The full list of subdivisions is available via `GET /data/dewey`.

## Document types

Registered types: **Notes**, **Book**, **Article**, **Report**, **Manual**, **Reference**.

Additional types can be registered in `main.go` by calling `docFactory.RegisterDocumentType(...)`.

Available types can be queried at runtime via `GET /data/types`.

## Testing

```bash
cd src/backend
go test ./...
```

Tests use temporary directories and isolated BoltDB instances — no external services required.

## Project structure detail

| Package | Responsibility |
|---|---|
| `dao` | Data Access Objects — BoltDB CRUD, document interfaces, MetaData struct, Dewey data, document factory |
| `fao` | File Access Objects — local filesystem read/write/delete |
| `converter` | Pandoc wrapper — format conversion by file path or document UUID |
| `service` | HTTP/gRPC handlers, service wrappers around DAO/FAO |
| `config` | Environment variable loading with defaults |

## License

See [LICENSE](LICENSE) if present.
