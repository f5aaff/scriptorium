package main

import (
    "fmt"
    "os"
    "scriptorium/internal/backend/dao"

    "github.com/google/uuid"
)

func main() {
    db := &dao.BoltDao{}
    err := db.Connect("/home/james/dev/practice/backend/low/go/scriptorium/scriptorium.db", 0600, nil)

    if err != nil {
        fmt.Println(err.Error())
    }
    if _, err := os.Stat("scriptorium.db"); os.IsNotExist(err) {
        fmt.Println("Database file does not exist!")
    } else {
        fmt.Println("Database file created successfully.")
    }

    meta := dao.MetaData{
        Title:       "test",
        Author:      "me",
        PublishDate: "right now",
        LastUpdated: "right now",
        FileType:    "md",
        Uuid:        uuid.New(),
    }
    // instantiate a Document interface
    var doc dao.Document

    // create concrete Notes struct
    note := dao.Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }

    // assign reference of concrete struct to the Document interface
    doc = &note

    // check that it actually worked
    fmt.Println(doc.GetTitle())
    fmt.Println(doc.GetMetaData())
    fmt.Println(doc.GetContent())
    fmt.Println(doc.GetID().String())

    // insert record into db
    err = db.Create(doc)
    if err != nil {
        fmt.Println(err)
    }

    // create empty Notes Document record
    var res dao.Document = &dao.Notes{}

    // read from the DB, with the provided UUID
    res, err = db.Read(&res, meta.Uuid)
    if err != nil {
        fmt.Println(err.Error())
    }

    // check it even worked
    fmt.Println(res.GetMetaData())

}
