# Scriptorium Svelte Frontend

The Svelte SPA that runs inside the Wails desktop shell (or standalone in a browser during development).

Built with **Svelte 3**, **TypeScript**, and **Vite 4**.

## Setup

```bash
npm install
```

## Development

```bash
npm run dev
```

Runs at `http://localhost:5173` with hot module replacement. Requires the Scriptorium backend to be running for API calls.

When running inside Wails (`wails dev`), the dev server is managed automatically.

## Build

```bash
npm run build
```

Output goes to `dist/`. Wails packages this automatically during `wails build`.

## Components

| File | Purpose |
|---|---|
| `App.svelte` | Root layout — sidebar, header, page routing |
| `Library.svelte` | Document grid with infinite scroll, fuzzy search, detail card with open/convert/edit/download/delete actions |
| `Add.svelte` | File upload form with drag-and-drop, image preview, document type dropdown, Dewey Decimal selector, metadata fields, validation |
| `EditModal.svelte` | Modal overlay for editing an existing document's metadata — fetches available types and Dewey categories from the API |
| `ItemCard.svelte` | Individual grid tile showing document title and file type |
| `Settings.svelte` | User preferences (theme, font size, language, sync, folder) persisted to `localStorage` |
| `Sidebar.svelte` | Navigation sidebar with links to Library, Add, and Settings |

## Configuration

`src/config.ts` exports `API_BASE_URL`, which defaults to `http://localhost:8080` and can be overridden with the `VITE_API_BASE_URL` environment variable.

## Search

The Library search bar supports fuzzy and prefix-based searching:

- **No prefix** — fuzzy match across title, author, type, Dewey code, file type
- `author:` — exact match on author
- `type:` — exact match on document type
- `dewey:` — exact match on Dewey Decimal code
- `filetype:` — exact match on file extension

## API Endpoints Used

| Endpoint | Used by |
|---|---|
| `GET /data/search` | Library (browse + search) |
| `GET /data/read/:uuid` | EditModal |
| `PUT /data/update` | EditModal |
| `DELETE /data/delete` | Library |
| `GET /data/types` | Add, EditModal |
| `GET /data/dewey` | Add, EditModal |
| `POST /file/upload` | Add |
| `GET /file/download/:uuid` | Library (open + download) |
| `GET /file/convert/:uuid` | Library (View as PDF) |
