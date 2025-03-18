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
    var dao dao.DAO = &dao.BoltDao{}
    err := dao.Connect(&conparams)
    if err != nil {
        log.Fatalf("error instantiated DB: %s", err.Error())
    }

    handler := service.NewAPIHandler(*service.New(dao))

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
