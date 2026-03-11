package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"scriptorium/internal/backend/config"
	"scriptorium/internal/backend/converter"
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
	docFactory.RegisterDocumentType("Book", func() dao.Document { return &dao.Notes{} })
	docFactory.RegisterDocumentType("Article", func() dao.Document { return &dao.Notes{} })
	docFactory.RegisterDocumentType("Report", func() dao.Document { return &dao.Notes{} })
	docFactory.RegisterDocumentType("Manual", func() dao.Document { return &dao.Notes{} })
	docFactory.RegisterDocumentType("Reference", func() dao.Document { return &dao.Notes{} })

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

	pandocConverter := converter.NewPandocConverterWithInterfaces("pandoc", d, f)
	_ = service.NewFileConverterService(pandocConverter, f) // registered for potential direct use

	apiHandler := service.NewAPIHandler(daos, docFactory, f)

	fileHandlerService := service.FileHandlerService{}
	fhServ, err := fileHandlerService.New(f)
	if err != nil {
		log.Fatalf("error instantiating FileHandlerService: %s", err.Error())
	}

	faos, ok := fhServ.(service.FileHandlerService)
	if !ok {
		log.Fatalf("error type checking FileHandlerService")
	}

	grpcServer := grpc.NewServer()
	grpcErrCh := service.StartGrcpService(grpcServer, faos, cfg.Server.GrpcPort)

	grpcAddr := fmt.Sprintf("localhost:%d", cfg.Server.GrpcPort)
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	fileHandler := service.NewFileHandler(faos, conn, apiHandler, pandocConverter)

	//---------------------------------------------------
	//-------------------SERVICE-START-------------------
	//---------------------------------------------------

	// Call StartRestAPI with handlers
	errCh := service.StartRestAPI(cfg.Server.RestPort, apiHandler, fileHandler)

	// Set up graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("REST API Error: %v", err)
		}
	case err := <-grpcErrCh:
		if err != nil {
			log.Fatalf("gRPC Error: %v", err)
		}
	case sig := <-signalCh:
		log.Printf("Received shutdown signal: %s", sig)
		grpcServer.GracefulStop()
		if err := d.Disconnect(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
		log.Println("Shutdown complete")
	}
}
