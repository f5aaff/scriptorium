package main

import (
    "log"
    "os"
    "os/signal"
    "scriptorium/internal/backend/dao"
    "scriptorium/internal/backend/service"
    "syscall"
)

func main() {
    var conparams dao.BoltConnectionParams = dao.BoltConnectionParams{Path: "./scriptorium.db", Mode: 0600, Opts: nil}
    var d dao.DAO = &dao.BoltDao{}
    err := d.Connect(&conparams)
    if err != nil {
        log.Fatalf("error instantiating DB: %s", err.Error())
    }

    docFactory := dao.NewDocumentFactory()
    docFactory.RegisterDocumentType("Notes", func() dao.Document { return &dao.Notes{} })

    daoService := service.DaoService{}
    serv, err := daoService.New(d)
    if err != nil {
        log.Fatalf("error instantiating DaoService: %s", err.Error())
    }
    daos, ok := serv.(service.DaoService)
    if !ok {
        log.Fatalf("error type checking DaoService")
    }
    handler := service.NewAPIHandler(daos,docFactory)

    // Call StartRestAPI with handlers
    errCh := service.StartRestAPI(handler)
    // Set up graceful shutdown
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

    // Handle errors in a non-blocking way
    select {
    case err := <-errCh:
        if err != nil {
            log.Fatalf("API Error: %v", err)
        }
    case sig := <-signalCh:
        log.Printf("Received shutdown signal: %s", sig)
        // Handle graceful shutdown
    }

}
