package dao

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/google/uuid"
)

var conparams *BoltConnectionParams = &BoltConnectionParams{Path: "./test.db", Mode: 0600, Opts: nil}

const tempDbPath = "./test.db"

func initDB() (*BoltDao, error) {
	db := BoltDao{}
	err := db.Connect(conparams)
	if err != nil {
		return nil, err
	}

	return &db, nil
}

func TestWhenCreateBoltDBExpectDbFile(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	if _, err := os.Stat(tempDbPath); os.IsNotExist(err) {
		t.Errorf("Temp file was not created properly")
	}
}

func TestWhenBoltDAOConnectExpectDB(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	if db.db == nil {
		t.Errorf("boltDao.db is nil, expected *bolt.DB")
	}
}

func TestWhenCreateTableExpectTable(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	meta := MetaData{
		Title:       "test",
		Author:      "me",
		PublishDate: "right now",
		LastUpdated: "right now",
		FileType:    "md",
		Uuid:        uuid.New().String(),
	}

	doc := &Notes{
		Title:    "test",
		Metadata: meta,
		Content:  "THIS IS A TEST DOCUMENT",
	}

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

func TestWhenCreateRecordExpectRecord(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	meta := MetaData{
		Title:       "test",
		Author:      "me",
		PublishDate: "right now",
		LastUpdated: "right now",
		FileType:    "md",
		Uuid:        uuid.New().String(),
	}

	doc := &Notes{
		Title:    "test",
		Metadata: meta,
		Content:  "THIS IS A TEST DOCUMENT",
	}

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

		data := bucket.Get([]byte(doc.GetID()))
		if data == nil {
			return fmt.Errorf("document not found")
		}

		return json.Unmarshal(data, &resMeta)
	})

	if err != nil {
		t.Errorf("error retrieving document: %s", err)
	}

	if resMeta.Uuid != meta.Uuid {
		t.Error("UUIDs do not match")
	}
}

func TestWhenSearchByKeyValueExpectRecords(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	meta := MetaData{
		Title:       "test",
		Author:      "me",
		PublishDate: "right now",
		LastUpdated: "right now",
		FileType:    "md",
		Uuid:        uuid.New().String(),
	}

	doc := &Notes{
		Title:    "test",
		Metadata: meta,
		Content:  "THIS IS A TEST DOCUMENT",
	}

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

func TestWhenDisconnectDBisNil(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}

	err = db.Disconnect()
	if err != nil {
		t.Errorf("error disconnecting from db file: %s", err)
	}

	if db.db != nil {
		t.Error("wanted boltDao.db == nil; have boltDao.db != nil")
	}
}

func TestWhenDeleteRecordExpectDeletedRecord(t *testing.T) {
	defer os.Remove(tempDbPath)

	db, err := initDB()
	if err != nil {
		t.Fatalf("error initialising DB: %s", err)
	}
	defer db.Disconnect()

	meta := MetaData{
		Title:       "test",
		Author:      "me",
		PublishDate: "right now",
		LastUpdated: "right now",
		FileType:    "md",
		Uuid:        uuid.New().String(),
	}

	doc := &Notes{
		Title:    "test",
		Metadata: meta,
		Content:  "THIS IS A TEST DOCUMENT",
	}

	err = db.Create(doc)
	if err != nil {
		t.Errorf("error inserting document: %s", err)
	}

	docUUID, err := uuid.Parse(doc.GetID())
	if err != nil {
		t.Errorf("error parsing UUID: %s", err)
	}

	err = db.Delete(docUUID)
	if err != nil {
		t.Errorf("error deleting document: %s", err)
	}

	var d Document = &Notes{}
	_, err = db.Read(&d, docUUID)
	if err == nil {
		t.Errorf("document found, deletion failed")
	}
}
