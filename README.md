# Scriptorium

Scriptorium intends to be a fairly flexible store of documents, using BoltDB to store information/metadata/location,
and a basic software architecture to allow the management, record, and retrieval of said documents.

# build
- ```go build .```

# dependencies
- go 1.23+


# Endpoints

## `/data` Endpoints

### ```/create``` **POST**
- This endpoint is used for inserting a document into the database
- **Request Body:**
```JSON
{
    "DocType": "Notes",
    "Title": "My Test Document",
    "Content": "This is the content of my test document",
    "MetaData": {
        "Title": "My Test Document",
        "Author": "John Doe",
        "PublishDate": "2024-01-15",
        "LastUpdated": "2024-01-15",
        "FileType": "txt",
        "DocType": "Notes",
        "Path": "./documents"
    }
}
```
- **Response:**
```JSON
{
    "message": "Document inserted into DB",
    "UUID": "7465e9fd-38db-4118-a6f1-d4ac0acce1e6"
}
```

### ```/read/:uuid``` **GET**
- This endpoint is for retrieving singular documents by their UUID
- **URL Parameter:** `:uuid` - The UUID of the document to retrieve
- **Example:**
```bash
curl http://localhost:8080/data/read/7465e9fd-38db-4118-a6f1-d4ac0acce1e6
```
- **Response:**
```JSON
{
    "message": "document retrieved",
    "value": "{\"Title\":\"My Test Document\",\"Author\":\"John Doe\",\"PublishDate\":\"2024-01-15\",\"LastUpdated\":\"2024-01-15\",\"FileType\":\"txt\",\"DocType\":\"Notes\",\"Path\":\"./documents\",\"Uuid\":\"7465e9fd-38db-4118-a6f1-d4ac0acce1e6\"}"
}
```

### ```/update``` **PUT**
- This endpoint is for updating existing documents
- **Request Body:**
```JSON
{
    "Title": "Updated Document",
    "Metadata": {
        "Title": "Updated Title",
        "Author": "Jane Doe",
        "PublishDate": "2025-03-19",
        "LastUpdated": "2025-03-19",
        "FileType": "txt",
        "Uuid": "550e8400-e29b-41d4-a716-446655440000"
    },
    "Content": "This is the updated content of the document."
}
```
- **Response:**
```JSON
{
    "message": "update successful",
    "value": "550e8400-e29b-41d4-a716-446655440000"
}
```

### ```/delete``` **DELETE**
- This endpoint is for deleting existing documents (supports bulk deletion)
- **Request Body:**
```JSON
{
    "uuids": [
        "7465e9fd-38db-4118-a6f1-d4ac0acce1e6",
        "12345678-1234-1234-1234-123456789abc"
    ]
}
```
- **Example:**
```bash
curl -X DELETE http://localhost:8080/data/delete \
  -H "Content-Type: application/json" \
  -d '{"uuids": ["7465e9fd-38db-4118-a6f1-d4ac0acce1e6"]}'
```
- **Response (Success):**
```JSON
{
    "deleted_count": 2,
    "deleted_uuids": [
        "7465e9fd-38db-4118-a6f1-d4ac0acce1e6",
        "12345678-1234-1234-1234-123456789abc"
    ]
}
```
- **Response (Partial Success):**
```JSON
{
    "deleted_count": 1,
    "deleted_uuids": ["7465e9fd-38db-4118-a6f1-d4ac0acce1e6"],
    "errors": ["Failed to delete UUID 'invalid-uuid': document not found"],
    "error_count": 1
}
```

### ```/search``` **GET**
- This endpoint is for searching documents with pagination support
- **Query Parameters:**
  - `key` (optional): Field to search in (e.g., "Title", "Author", "DocType")
  - `value` (optional): Value to search for
  - `page` (optional, default: 1): Page number (positive integer)
  - `limit` (optional, default: 10): Results per page (1-100)

#### Search Examples:

**Get all documents (first page):**
```bash
curl "http://localhost:8080/data/search"
```

**Get all documents with pagination:**
```bash
curl "http://localhost:8080/data/search?page=2&limit=5"
```

**Search by author:**
```bash
curl "http://localhost:8080/data/search?key=Author&value=John%20Doe&page=1&limit=10"
```

**Search by document type:**
```bash
curl "http://localhost:8080/data/search?key=DocType&value=Notes&page=1&limit=20"
```

**Search by title:**
```bash
curl "http://localhost:8080/data/search?key=Title&value=My%20Document&page=1&limit=10"
```

- **Response:**
```JSON
{
    "message": "Search completed",
    "count": 5,
    "total_count": 25,
    "page": 1,
    "limit": 10,
    "total_pages": 3,
    "has_next": true,
    "has_prev": false,
    "results": [
        {
            "Title": "My Test Document",
            "Author": "John Doe",
            "PublishDate": "2024-01-15",
            "LastUpdated": "2024-01-15",
            "FileType": "txt",
            "DocType": "Notes",
            "Path": "./documents",
            "Uuid": "7465e9fd-38db-4118-a6f1-d4ac0acce1e6"
        }
    ]
}
```

## `/file` Endpoints

### ```/upload``` **POST**
- Upload files to the system
- **Request:** Multipart form data with file field
- **Example:**
```bash
curl -X POST http://localhost:8080/file/upload \
  -F "file=@/path/to/your/file.txt"
```

### ```/download/:filename``` **GET**
- Download files from the system
- **URL Parameter:** `:filename` - The name of the file to download
- **Example:**
```bash
curl http://localhost:8080/file/download/myfile.txt
```

## Available Search Fields

When using the search endpoint, you can search by any of these metadata fields:
- `Title`
- `Author`
- `PublishDate`
- `LastUpdated`
- `FileType`
- `DocType`
- `Path`
- `Uuid`

## URL Encoding

When using search parameters with spaces or special characters, remember to URL encode them:
- Space: `%20` or `+`
- Special characters: Use proper URL encoding

**Example:**
```bash
# Search for "My Test Document" (space encoded as %20)
curl "http://localhost:8080/data/search?key=Title&value=My%20Test%20Document"
```

