package main

import (
	"log"
	"os"
	"os/signal"
	"scriptorium/internal/backend/config"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/service"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	//---------------------------------------------------
	//----------------API-HANDLER-SET-UP-----------------
	//---------------------------------------------------

	var conparams dao.BoltConnectionParams = dao.BoltConnectionParams{Path: cfg.Database.Path, Mode: os.FileMode(cfg.Database.Mode), Opts: nil}
	var d dao.DAO = &dao.BoltDao{}
	err = d.Connect(&conparams)
	if err != nil {
		log.Fatalf("error instantiating DB: %s", err.Error())
	}

	docFactory := dao.NewDocumentFactory()
	docFactory.RegisterDocumentType("Notes", func() dao.Document { return &dao.Notes{} })

	daoService := service.DaoService{}
	daoServ, err := daoService.New(d)
	if err != nil {
		log.Fatalf("error instantiating DaoService: %s", err.Error())
	}
	daos, ok := daoServ.(service.DaoService)
	if !ok {
		log.Fatalf("error type checking DaoService")
	}

	//---------------------------------------------------
	//----------------FILE-HANDLER-SET-UP----------------
	//---------------------------------------------------

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(cfg.Storage.Path, 0755); err != nil {
		log.Fatalf("failed to create storage directory: %v", err)
	}

	f := fao.NewLocalFao(cfg.Storage.Path)

	apiHandler := service.NewAPIHandler(daos, docFactory, f)

	fileHandlerService := service.FileHandlerService{}
	fhServ, err := fileHandlerService.New(f)

	faos, ok := fhServ.(service.FileHandlerService)
	if !ok {
		log.Fatalf("error type checking FileHandlerService")
	}

	grpcServer := grpc.NewServer()
	grpcErrCh := service.StartGrcpService(grpcServer, faos)
	grpcSignalCh := make(chan os.Signal, 1)
	signal.Notify(grpcSignalCh, syscall.SIGINT, syscall.SIGTERM)

	conn, err := grpc.NewClient("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	fileHandler := service.NewFileHandler(faos, conn, apiHandler)

	//---------------------------------------------------
	//-------------------SERVICE-START-------------------
	//---------------------------------------------------

	// Call StartRestAPI with handlers
	errCh := service.StartRestAPI(apiHandler, fileHandler)
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

	case err := <-grpcErrCh:
		if err != nil {
			log.Fatalf("API Error: %v", err)
		}
	case sig := <-grpcSignalCh:
		log.Printf("Received shutdown signal: %s", sig)
		// Handle graceful shutdown
	}
}
