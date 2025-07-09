package dao

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"slices"

	"github.com/boltdb/bolt"
	"github.com/google/uuid"
)

//---------------------------------------------------
//----------------------DAO--------------------------
//---------------------------------------------------

type ConnectParams interface {
	getParams() any
}

// DAO interface, contains basic CRUD functions
type DAO interface {
	Create(Document) error
	Read(*Document, uuid.UUID) (Document, error)
	ReadRaw(uuid.UUID) ([]byte, error)
	SearchByKeyValue(key, value string) ([]MetaData, error)
	Update(Document) error
	Delete(uuid.UUID) error
	Connect(ConnectParams) error
	Disconnect() error
}

//---------------------------------------------------
//-------------------BOLT-DAO------------------------
//---------------------------------------------------

type BoltConnectionParams struct {
	Path string
	Mode fs.FileMode
	Opts *bolt.Options
}

// TODO: fix this dumpster fire of a design decision
// this is nightmare fuel, and needs a more elegant solution -
// perhaps interface/concrete isn't necessarily the right tool for the job?
func (bcp *BoltConnectionParams) getParams() any {
	return *bcp
}

// BoltDAO struct, with realised methods from the DAO interface
type BoltDao struct {
	db *bolt.DB
}

func (b *BoltDao) Connect(cp ConnectParams) error {
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist.
	params, ok := cp.getParams().(BoltConnectionParams)
	if !ok {
		return fmt.Errorf("connection parameters do not conform to BoltConnectionParams type.")
	}
	// connect to given db
	// TODO: template out db path and permissions to environment variables
	db, err := bolt.Open(params.Path, params.Mode, params.Opts)
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

		docID := []byte(doc.GetID())
		docData, err := json.Marshal(metaData)
		if err != nil {
			return fmt.Errorf("could not insert document: %v", err)
		}

		return bucket.Put(docID, docData)
	})
	return err
}

func (b *BoltDao) ReadRaw(id uuid.UUID) ([]byte, error) {
	var rawData []byte

	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("documents"))
		if bucket == nil {
			return fmt.Errorf("documents bucket does not exist")
		}

		data := bucket.Get([]byte(id.String()))
		if data == nil {
			return fmt.Errorf("document not found")
		}

		rawData = slices.Clone(data) // Copy data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving document: %v", err)
	}
	return rawData, nil
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

		docID := []byte(doc.GetID())
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

func (b *BoltDao) SearchByKeyValue(key, value string) ([]MetaData, error) {
	var results []MetaData

	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("documents"))
		if bucket == nil {
			return fmt.Errorf("documents bucket does not exist")
		}

		c := bucket.Cursor()
		for _, v := c.First(); v != nil; _, v = c.Next() {
			var metaData MetaData
			if err := json.Unmarshal(v, &metaData); err != nil {
				return fmt.Errorf("error unmarshaling document: %v", err)
			}

			// Check if metadata contains the key-value pair (case-insensitive match for strings)
			if metaDataMatches(metaData, key, value) {
				results = append(results, metaData)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error searching documents: %v", err)
	}

	return results, nil
}

// Helper function to check if metadata struct contains the key-value pair
func metaDataMatches(metaData MetaData, key, value string) bool {
	metaValue, err := getStructFieldValue(metaData, key)
	if err != nil {
		return false // Field not found or invalid
	}
	return metaValue == value
}

// Reflection-based function to retrieve a field value by name
func getStructFieldValue(metaData MetaData, fieldName string) (string, error) {
	val := reflect.ValueOf(metaData)

	// Ensure we're dealing with a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem() // Dereference pointer if needed
	}

	if val.Kind() != reflect.Struct {
		return "", fmt.Errorf("expected struct, got %s", val.Kind())
	}

	field := val.FieldByName(fieldName)
	if !field.IsValid() {
		return "", fmt.Errorf("field %s not found in metadata", fieldName)
	}

	// Ensure the field is a string before returning it
	if field.Kind() == reflect.String {
		return field.String(), nil
	}

	return "", fmt.Errorf("field %s is not a string", fieldName)
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
	DocType     string
	Path        string
	Uuid        string
}

//---------------------------------------------------
//-----------------DOCUMENT-FACTORY------------------
//---------------------------------------------------

// DocumentFactoryFunc defines a function signature for document creation.
type DocumentFactoryFunc func() Document

// DocumentFactory encapsulates the registry for document types.
type DocumentFactory struct {
	registry map[string]DocumentFactoryFunc
}

// NewDocumentFactory creates an instance of the factory.
func NewDocumentFactory() *DocumentFactory {
	return &DocumentFactory{
		registry: make(map[string]DocumentFactoryFunc),
	}
}

// RegisterDocumentType registers a document type in the factory.
func (f *DocumentFactory) RegisterDocumentType(docType string, factory DocumentFactoryFunc) {
	f.registry[docType] = factory
}

// NewDocument dynamically creates a document instance.
func (f *DocumentFactory) NewDocument(docType string) (Document, error) {
	if factory, found := f.registry[docType]; found {
		return factory(), nil
	}
	return nil, fmt.Errorf("unknown document type: %s", docType)
}

//---------------------------------------------------
//---------------------DOCUMENT----------------------
//---------------------------------------------------

// over-arching interface to cover all subsequent document types,
// supports some quality of life methods, and getting/setting metadata
type Document interface {
	GetTitle() string
	SetTitle(string) error
	GetMetaData() MetaData
	SetMetaData(MetaData) error
	GetID() string
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

func (n *Notes) SetTitle(title string) error {
	n.Title = title
	return nil
}

func (n Notes) GetMetaData() MetaData {
	return n.Metadata
}

func (n *Notes) SetMetaData(meta MetaData) error {
	n.Metadata = meta
	return nil
}

func (n *Notes) GetContent() any {
	return n.Content
}

func (n *Notes) SetContent(content any) error {
	strContent, ok := content.(string)
	if !ok {
		return fmt.Errorf("error setting content, check content type")
	}
	n.Content = strContent
	return nil
}

func (n *Notes) GetID() string {
	return n.Metadata.Uuid
}
