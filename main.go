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
    var doc dao.Document = &dao.Notes{}
    note := dao.Notes{
        Title:    "test",
        Metadata: meta,
        Content:  "THIS IS A TEST DOCUMENT, AAAAAAAAAAA",
    }
    doc = &note
    fmt.Println(doc.GetTitle())
    fmt.Println(doc.GetMetaData())
    fmt.Println(doc.GetContent())
    fmt.Println(doc.GetID().String())

    err = db.Create(doc)
    if err != nil {
        fmt.Println(err)
    }
    var res dao.Document = &dao.Notes{}

    res,err = db.Read(&res,meta.Uuid)
    if err != nil {
        fmt.Println(err.Error())
    }
    fmt.Println(res.GetMetaData())

}
