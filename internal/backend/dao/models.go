package dao

import (
    "encoding/json"
    "fmt"
    "io/fs"

    "github.com/boltdb/bolt"
    "github.com/google/uuid"
)

//---------------------------------------------------
//----------------------DAO--------------------------
//---------------------------------------------------

// DAO interface, contains basic CRUD functions
type DAO interface {
    Create(Document) error
    Read(Document, uuid.UUID) (Document,error)
    Update(Document) error
    Delete(uuid.UUID) error
    // TODO: this needs it's params abstracted out into a class, to maintain inheritance.
    Connect() error
    Disconnect() error
}

// BoltDAO struct, with realised methods from the DAO interface
type BoltDao struct {
    db *bolt.DB
}

func (b *BoltDao) Create(doc Document) error {
    metaData := doc.GetMetaData()

    err := b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            var err error
            bucket, err = tx.CreateBucket([]byte("documents"))
            if err != nil {
                return fmt.Errorf("could not create documents bucket: %v", err)
            }
        }

        docID := []byte(doc.GetID().String())
        docData, err := json.Marshal(metaData)
        if err != nil {
            return fmt.Errorf("could not insert document: %v", err)
        }

        return bucket.Put(docID, docData)
    })
    return err
}

// Read method for BoltDao, expects a Document(empty ideally, for example, a 'Note') and a UUID
func (b *BoltDao) Read(doc *Document, id uuid.UUID) (Document, error) {
    var metaData MetaData

    // use View to retrieve from documents bucket, erroring if it doesn't exist
    err := b.db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            return fmt.Errorf("documents bucket does not exist")
        }

        // get by UUID
        data := bucket.Get([]byte(id.String()))
        if data == nil {
            return fmt.Errorf("docment not found")
        }

        // unmarshal the metadata from the JSON response
        return json.Unmarshal(data, &metaData)
    })
    if err != nil {
        return nil, fmt.Errorf("error retrieving document: %v", err)
    }

    // call the Documents setMetaData method, by dereferencing the Document
    err = (*doc).SetMetaData(metaData)
    if err != nil {
        return nil, fmt.Errorf("error reading metaData: %v", err)
    }
    // return the dereferenced Document
    return *doc, nil
}

func (b *BoltDao) Update(doc Document) error {
    metaData := doc.GetMetaData()

    err := b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            return fmt.Errorf("documents bucket does not exist")
        }

        docID := []byte(doc.GetID().String())
        docData, err := json.Marshal(metaData)
        if err != nil {
            return fmt.Errorf("could not update document: %v", err)
        }

        return bucket.Put(docID, docData)
    })
    return err
}

func (b *BoltDao) Delete(id uuid.UUID) error {
    err := b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            return fmt.Errorf("documents bucket does not exist")
        }

        return bucket.Delete([]byte(id.String()))
    })
    return err
}

func (b *BoltDao) Connect(path string, mode fs.FileMode, opts *bolt.Options) error {
    // Open the my.db data file in your current directory.
    // It will be created if it doesn't exist.

    // connect to given db
    // TODO: template out db path and permissions to environment variables
    db, err := bolt.Open(path, mode, opts)
    if err != nil {
        return fmt.Errorf("failed to open DB: %v", err)
    }

    // assign DAO db to established connection
    b.db = db
    return nil
}

func (b *BoltDao) Disconnect() error {
    if b.db != nil {
        err := b.db.Close()
        if err != nil {
            return fmt.Errorf("failed to close DB: %v", err)
        }
        b.db = nil
    }
    return nil
}

//---------------------------------------------------
//---------------------METADATA----------------------
//---------------------------------------------------

// basic struct, contains generic information
// regarding the document it references.
type MetaData struct {
    Title       string
    Author      string
    PublishDate string
    LastUpdated string
    FileType    string
    Uuid        uuid.UUID
}

//---------------------------------------------------
//---------------------DOCUMENT----------------------
//---------------------------------------------------

// over-arching interface to cover all subsequent document types,
// supports some quality of life methods, and getting/setting metadata and content.
type Document interface {
    GetTitle() string
    GetMetaData() MetaData
    SetMetaData(MetaData) error
    GetContent() interface{}
    SetContent(interface{}) error
    GetID() uuid.UUID
}

// basic type, for functionality testing. content is just a simple string.
type Notes struct {
    Title    string
    Metadata MetaData
    Content  string
}

func (n Notes) GetTitle() string {
    return n.Title
}

func (n Notes) GetMetaData() MetaData {
    return n.Metadata
}

func (n *Notes) SetMetaData(meta MetaData) error {
    n.Metadata = meta
    return nil
}

func (n *Notes) GetContent() interface{} {
    return n.Content
}

func (n *Notes) SetContent(content interface{}) error {
    strContent, ok := content.(string)
    if !ok {
        return fmt.Errorf("error setting content, check content type")
    }
    n.Content = strContent
    return nil
}

func (n *Notes) GetID() uuid.UUID {
    return n.Metadata.Uuid
}
