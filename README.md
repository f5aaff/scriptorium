# Scriptorium

Scriptorium intends to be a fairly flexible store of documents, using BoltDB to store information/metadata/location,
and a basic software architecture to allow the management, record, and retrieval of said documents.

# build
- ```go build .```

# dependencies
- go 1.23+


# Endpoints

- ```/data```
    - ```/create``` **POST**
        - this endpoint is used for inserting a document into the database
        - expects JSON of the following structure:
        ```JSON
            {
               "Title": "some title",
               "MetaData": {
                   "Title": "some title",
                   "Author": "joe blogs"
                   "PublishDate": "01-01-1970" 
                   "LastUpdated": "01-01-1980" 
                   "FileType": "pdf" 
                   "DocType": "Notes" 
                   "Path": "./notes" 
               }
            }
        ```
        - response:
        ```JSON
            {
                "message": "Document inserted into DB",
                "UUID": "<UUID of the document, after insertion>"
            }
        ```
    - ```/read``` **POST**
        - this endpoint is for retrieving singular documents, via their UUID.
        - expects JSON of the following structure:
        ```JSON
            {"uuid": "<Document UUID>"}
        ```
        - response:
        ```JSON
            {
               "message" : "document retrieved",
                   "value": {
                       "Title": "some title",
                       "MetaData": {
                           "Title": "some title",
                           "Author": "joe blogs"
                           "PublishDate": "01-01-1970"
                           "LastUpdated": "01-01-1980"
                           "FileType": "pdf"
                           "DocType": "Notes"
                           "Path": "./notes"
                           "Uuid": "<some uuid here, as a string>"
                       }
                   }
            }
        ```
    - ```/update``` **PUT**
        - this endpoint is for updating existing documents.
        - expects JSON of the following structure:
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
    - ```/delete``` **POST**
        - this endpoint is for deleting existing documents.
        - expects JSON of the following structure:
        ```JSON
            {
                "uuids": ["uuid1","uuid2"...]
            }
        ```
