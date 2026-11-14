# Scriptorium Frontend

Wails v2 desktop application wrapping a Svelte SPA. Provides a native window with Go backend bindings for features like native file/folder dialogs.

## Prerequisites

- **Go** 1.23+
- **Node.js** 18+ and npm
- **Wails CLI** v2 — install with `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- The Scriptorium REST backend must be running (default `127.0.0.1:8080`) for data and file operations. File transfers use the REST API and local storage; the old gRPC listener has been removed.

## Project Structure

```
src/frontend/
├── app.go              # Wails Go backend — exposes SelectFolder() to the UI
├── main.go             # Wails app entry point
├── wails.json          # Wails project configuration
├── build/              # Platform-specific build assets (icons, manifests)
└── frontend/           # Svelte SPA (see frontend/README.md)
    ├── index.html
    ├── package.json
    ├── vite.config.ts
    └── src/
        ├── App.svelte
        ├── config.ts
        └── components/
```

## Development

```bash
cd src/frontend
wails dev
```

This starts a Vite dev server with hot reload inside a native desktop window. A browser-accessible dev server also runs at `http://localhost:34115` for debugging with devtools.

### Frontend-only development

If you only want to work on the Svelte code without the Wails wrapper:

```bash
cd src/frontend/frontend
npm install
npm run dev
```

This runs the Svelte app in the browser at `http://localhost:5173`. Set `VITE_API_BASE_URL` to the backend URL when the frontend and API use different origins, for example `VITE_API_BASE_URL=http://192.168.1.20:8080 npm run dev -- --host 0.0.0.0`.

## Building

Build the desktop application:

```bash
cd src/frontend
wails build
```

Build the browser client for hosting by the Go backend:

```bash
cd src/frontend/frontend
npm run build
mkdir -p ../../backend/web
cp -a dist/. ../../backend/web/
```

Then run the backend from `src/backend`; it serves the SPA from `WEB_ROOT` (default `./web`) on the same origin as the API.

The compiled binary is output to `build/bin/`. Platform-specific assets (icons, manifests, plist files) live in the `build/` subdirectories and can be customised.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `VITE_API_BASE_URL` | `http://localhost:8080` | Backend API URL used by the Svelte frontend |

Set this in a `.env` file in the `frontend/` directory or as an environment variable before building.

## Go Bindings

The Wails Go backend (`app.go`) exposes functions callable from JavaScript:

| Function | Description |
|---|---|
| `SelectFolder()` | Opens a native directory picker dialog, returns the selected path |

These are auto-generated into `frontend/src/wailsjs/` by Wails and can be imported in Svelte components.

## Views

| Component | Description |
|---|---|
| **Library** | Browse, search (fuzzy + prefix filters), open, convert to PDF, edit metadata, download, delete |
| **Add** | Upload files with metadata — document type dropdown, Dewey Decimal selector, image preview, validation |
| **EditModal** | Modal for editing document metadata (title, author, type, Dewey, publish date) |
| **Settings** | Preferences persisted to localStorage — theme, font size, language, sync, folder selection |
| **Sidebar** | Navigation between views |
