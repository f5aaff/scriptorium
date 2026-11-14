package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"scriptorium/internal/backend/auth"
	"scriptorium/internal/backend/config"
	"scriptorium/internal/backend/converter"
	"scriptorium/internal/backend/dao"
	"scriptorium/internal/backend/fao"
	"scriptorium/internal/backend/ingest"
	"scriptorium/internal/backend/service"
	"syscall"
	"time"
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

	migrated, err := dao.MigrateBoltToSQLite(cfg.Database.Path, os.FileMode(cfg.Database.Mode))
	if err != nil {
		log.Fatalf("error migrating legacy catalogue: %v", err)
	}
	if migrated {
		log.Printf("migrated legacy BoltDB catalogue to SQLite; backup retained at %s.bolt.bak", cfg.Database.Path)
	}
	var conparams = dao.SQLiteConnectionParams{Path: cfg.Database.Path, Mode: os.FileMode(cfg.Database.Mode)}
	var d dao.DAO = &dao.SQLiteDAO{}
	err = d.Connect(&conparams)
	if err != nil {
		log.Fatalf("error instantiating DB: %s", err.Error())
	}
	defer func() {
		if err := d.Disconnect(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	docFactory := dao.NewDocumentFactory()
	docFactory.RegisterDocumentType("File", func() dao.Document { return &dao.Notes{} })
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
	if err := os.MkdirAll(cfg.Storage.Path, 0700); err != nil {
		log.Fatalf("failed to create storage directory: %v", err)
	}

	f := fao.NewLocalFao(cfg.Storage.Path)

	pandocConverter := converter.NewPandocConverterWithInterfaces("pandoc", d, f)
	_ = service.NewFileConverterService(pandocConverter, f) // registered for potential direct use

	apiHandler := service.NewAPIHandler(daos, docFactory, f)

	fileHandler := service.NewFileHandler(f, apiHandler, pandocConverter)
	jobRepository, ok := d.(ingest.JobRepository)
	if !ok {
		log.Fatalf("configured database does not support durable ingestion jobs")
	}
	jobManager, err := ingest.NewJobManager(jobRepository, fileHandler.Ingestor, filepath.Join(cfg.Storage.Path, ".jobs"), cfg.Import.Roots, ingest.DefaultJobWorkers)
	if err != nil {
		log.Fatalf("failed to initialise ingestion jobs: %v", err)
	}
	jobManager.Start(context.Background())
	fileHandler.ConfigureJobs(jobManager)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := jobManager.Stop(ctx); err != nil {
			log.Printf("ingestion worker shutdown: %v", err)
		}
	}()

	//---------------------------------------------------
	//-------------------SERVICE-START-------------------
	//---------------------------------------------------

	authManager, err := auth.NewManager(cfg.Auth.File, cfg.Auth.Username, cfg.Auth.Password, cfg.Auth.CookieSecure)
	if err != nil {
		log.Fatalf("failed to initialise authentication: %v", err)
	}

	server, err := service.NewHTTPServer(cfg.Server.RestHost, cfg.Server.RestPort, cfg.Server.WebRoot, authManager, cfg.Server.AllowedOrigins, cfg.Server.TrustedProxies, apiHandler, fileHandler)
	if err != nil {
		log.Fatalf("failed to configure HTTP server: %v", err)
	}
	errCh := make(chan error, 1)
	log.Printf("Scriptorium API listening on %s", server.Addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Set up graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Printf("HTTP server failed: %v", err)
	case sig := <-signalCh:
		log.Printf("Received shutdown signal: %s", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful HTTP shutdown failed: %v", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("HTTP close failed: %v", closeErr)
			}
		}
		log.Println("Shutdown complete")
	}
}
