package service

import (
	"fmt"
	"scriptorium/internal/backend/converter"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"strings"

	"github.com/google/uuid"
)

type Service interface {
	New(any) (Service, error)
}

//---------------------------------------------------
//--------------FILE-CONVERTER-SERVICE---------------
//---------------------------------------------------

type FileConverterService struct {
	converter converter.Converter
	fao       fao.FAO
}

func NewFileConverterService(conv converter.Converter, f fao.FAO) *FileConverterService {
	return &FileConverterService{converter: conv, fao: f}
}

func (fcs FileConverterService) New(f any) (Service, error) {
	converter, ok := f.(converter.Converter)
	if !ok {
		return nil, fmt.Errorf("f is not a valid Converter")
	}
	fc := FileConverterService{converter: converter}
	return fc, nil
}

func (fcs FileConverterService) Convert(file dao.MetaData, toFormat string) (string, error) {
	exists := fcs.fao.FileExists(file.Path)
	if !exists {
		return "", fmt.Errorf("%s does not exist", file.Path)
	}

	fromFormat := strings.TrimPrefix(file.FileType, ".")
	outputPath, err := fcs.converter.ConvertFileByPath(file.Path, fromFormat, toFormat)
	if err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}
	return outputPath, nil
}

//---------------------------------------------------
//-------------------DAO-SERVICE---------------------
//---------------------------------------------------

// this service is fairly bare-bones, as is more just a layer of abstraction for now,
// intending that when a service is instantiated, the only thing that will change is the
// underlying DAO type.
type DaoService struct {
	dao dao.DAO
}

func (ds DaoService) New(d any) (Service, error) {
	dao, ok := d.(dao.DAO)
	if !ok {
		return nil, fmt.Errorf("d is not a valid DAO")
	}

	daos := DaoService{dao: dao}
	return daos, nil
}

func (ds *DaoService) SearchByKeyValue(key, value string) ([]dao.MetaData, error) {
	if key == "" && value == "" {
		docs, err := ds.dao.GetAll()
		if err != nil {
			return nil, err
		}
		return docs, nil
	}
	docs, err := ds.dao.SearchByKeyValue(key, value)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func (ds *DaoService) FuzzySearch(query string) ([]dao.MetaData, error) {
	return ds.dao.FuzzySearch(query)
}

func (ds *DaoService) Connect(params dao.ConnectParams) error {
	return ds.dao.Connect(params)
}

func (ds *DaoService) Disconnect() error {
	return ds.dao.Disconnect()
}

func (ds *DaoService) Create(doc dao.Document) error {
	return ds.dao.Create(doc)
}

// basically defunct until I can somehow wrangle this to work
func (ds *DaoService) Read(doc *dao.Document, uuid uuid.UUID) (dao.Document, error) {
	return ds.dao.Read(doc, uuid)
}

func (ds *DaoService) ReadRaw(uuid uuid.UUID) ([]byte, error) {
	return ds.dao.ReadRaw(uuid)
}
func (ds *DaoService) Update(doc dao.Document) error {
	return ds.dao.Update(doc)
}

func (ds *DaoService) Delete(id uuid.UUID) error {
	return ds.dao.Delete(id)
}
