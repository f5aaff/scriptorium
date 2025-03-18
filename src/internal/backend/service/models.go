package service

import (
    "scriptorium/internal/backend/dao"
    "github.com/google/uuid"
)

type Service interface{
    New(any)any
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

func New(dao dao.DAO) *DaoService {
    return &DaoService{dao: dao}
}

func (ds *DaoService) SearchByKeyValue(key, value string) ([]dao.MetaData, error) {
    docs, err := ds.dao.SearchByKeyValue(key, value)
    if err != nil {
        return nil, err
    }
    return docs, nil
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

func (ds *DaoService) Read(doc *dao.Document, uuid uuid.UUID) (dao.Document, error) {
    return ds.dao.Read(doc, uuid)
}

func (ds *DaoService) Update(doc dao.Document) error {
    return ds.dao.Update(doc)
}

func (ds *DaoService) Delete(id uuid.UUID) error {
    return ds.dao.Delete(id)
}



