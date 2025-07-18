package main

import (
	"log"
	"os"
	"os/signal"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/service"
	"scriptorium/internal/backend/service/pb"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	//---------------------------------------------------
	//----------------API-HANDLER-SET-UP-----------------
	//---------------------------------------------------

	var conparams dao.BoltConnectionParams = dao.BoltConnectionParams{Path: "./scriptorium.db", Mode: 0600, Opts: nil}
	var d dao.DAO = &dao.BoltDao{}
	err := d.Connect(&conparams)
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

	apiHandler := service.NewAPIHandler(daos, docFactory)

	//---------------------------------------------------
	//----------------FILE-HANDLER-SET-UP----------------
	//---------------------------------------------------

	f := fao.NewLocalFao("./storage")

	grpcServer := grpc.NewServer()

	conn, err := grpc.NewClient("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	fileHandlerService := service.FileHandlerService{}
	fhServ, err := fileHandlerService.New(f)

	pb.RegisterFileServiceServer(grpcServer, fileHandlerService)
	if err != nil {
		log.Fatalf("error instantiating FileHandlerService: %s", err.Error())
	}

	faos, ok := fhServ.(service.FileHandlerService)
	if !ok {
		log.Fatalf("error type checking FileHandlerService")
	}

	fileHandler := service.NewFileHandler(faos, conn)

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
	}

}
