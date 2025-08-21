package dao

import (
    "encoding/json"
    "fmt"
    "os"
    "testing"

    "github.com/boltdb/bolt"
    "github.com/google/uuid"
)

// connection parameters for connection to test DB
var conparams *BoltConnectionParams = &BoltConnectionParams{Path: "./test.db", Mode: 0600, Opts: nil}

// path to make/access test db
const tempDbPath = "./test.db"

func initDB() (*BoltDao, error) {
    db := BoltDao{}
    err := db.Connect(conparams)
    if err != nil {
        return nil, err
    }

    return &db, nil
}

// test that when a connection is made, and no file exists, one is created.
func TestWhenCreateBoltDBExpectDbFile(t *testing.T) {

    defer os.Remove(tempDbPath)

    _, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }
    if _, err := os.Stat(tempDbPath); os.IsNotExist(err) {
        t.Errorf("Temp file was not created properly")
    }
}

// test that the BoltDB can create a db and connect to it.
func TestWhenBoltDAOConnectExpectDB(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }
    if db.db != nil {
        t.Errorf("boltDao.db is null, expected *bolt.DB")
    }
}

// when a connection is made, and a create query is sent - a table should be created.
func TestWhenCreateTableExpectTable(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }

    meta := MetaData{
        Title:       "test",
        Author:      "me",
        PublishDate: "right now",
        LastUpdated: "right now",
        FileType:    "md",
        Uuid:        uuid.New().String(),
    }
    // instantiate a Document interface
    var doc Document

    // create concrete Notes struct
    note := Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }
    fmt.Println("\n___retrieve values from Document interface___")
    // assign reference of concrete struct to the Document interface
    doc = &note

    // insert record into db
    err = db.Create(doc)
    if err != nil {
        t.Errorf("error inserting document: %s", err)
    }

    err = db.db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            return fmt.Errorf("documents bucket does not exist")
        }
        return nil
    })

    if err != nil {
        t.Errorf("bucket not found: %s", err)
    }
}

// test that a dao with an active connection can make entries in the db.
func TestWhenCreateRecordExpectRecord(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }

    meta := MetaData{
        Title:       "test",
        Author:      "me",
        PublishDate: "right now",
        LastUpdated: "right now",
        FileType:    "md",
        Uuid:        uuid.New().String(),
    }
    // instantiate a Document interface
    var doc Document

    // create concrete Notes struct
    note := Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }
    fmt.Println("\n___retrieve values from Document interface___")
    // assign reference of concrete struct to the Document interface
    doc = &note

    // insert record into db
    err = db.Create(doc)
    if err != nil {
        t.Errorf("error inserting document: %s", err)
    }
    resMeta := MetaData{}
    err = db.db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("documents"))
        if bucket == nil {
            return fmt.Errorf("documents bucket does not exist")
        }

        // get by UUID
        data := bucket.Get([]byte(doc.GetID()))
        if data == nil {
            return fmt.Errorf("docment not found")
        }

        // unmarshal the metadata from the JSON response
        return json.Unmarshal(data, &resMeta)
    })

    if err != nil {
        t.Errorf("error retrieving document: %s", err)
    }

    if resMeta.Uuid != meta.Uuid {
        t.Error("UUIDs do not match")
    }
}

// test that search functionality works against key/value pairs
func TestWhenSearchByKeyValueExpectRecords(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }

    meta := MetaData{
        Title:       "test",
        Author:      "me",
        PublishDate: "right now",
        LastUpdated: "right now",
        FileType:    "md",
        Uuid:        uuid.New().String(),
    }
    // instantiate a Document interface
    var doc Document

    // create concrete Notes struct
    note := Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }
    fmt.Println("\n___retrieve values from Document interface___")
    // assign reference of concrete struct to the Document interface
    doc = &note

    // insert record into db
    err = db.Create(doc)
    if err != nil {
        t.Errorf("error inserting document: %s", err)
    }

    metas, err := db.SearchByKeyValue("Title", "test")
    if err != nil {
        t.Errorf("error searching by Key-Value pair: %s", err)
    }

    if len(metas) == 0 {
        t.Errorf("no records retrieved")
    }

    empty := metas[len(metas)-1] == MetaData{}
    if empty {
        t.Errorf("metadata empty")
    }
}

// the DAO 'db' field is a pointer, and upon disconnection should be nullified.
func TestWhenDisconnectDBisNil(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }

    err = db.Disconnect()

    if err != nil {
        t.Errorf("error disconnecting from db file: %s", err)
    }

    if db.db != nil {
        t.Error("wanted boltDao.db == nil; have boltDao.db != nil")
    }

}

// check that upon deletion being called, the record is successfully removed.
func TestWhenDeleteRecordExpectDeletedRecord(t *testing.T) {

    defer os.Remove(tempDbPath)

    db, err := initDB()
    if err != nil {
        t.Errorf("error initialising DB: %s", err)
    }

    err = db.Connect(conparams)
    if err != nil {
        t.Errorf("failed to create %s", tempDbPath)
    }

    meta := MetaData{
        Title:       "test",
        Author:      "me",
        PublishDate: "right now",
        LastUpdated: "right now",
        FileType:    "md",
        Uuid:        uuid.New().String(),
    }
    // instantiate a Document interface
    var doc Document

    // create concrete Notes struct
    note := Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }
    fmt.Println("\n___retrieve values from Document interface___")
    // assign reference of concrete struct to the Document interface
    doc = &note

    // insert record into db
    err = db.Create(doc)
    if err != nil {
        t.Errorf("error inserting document: %s", err)
    }

    var d Document = &Notes{}
    uuid,err := uuid.Parse(doc.GetID())
    if err != nil {
        t.Errorf("error parsing UUID: %s",err)
    }
    _, err = db.Read(&d, uuid)
    if err == nil {
        t.Errorf("document found, deletion failed")
    }
}
