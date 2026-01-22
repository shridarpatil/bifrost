package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"

	"github.com/shridarpatil/bifrost/config"
	"github.com/shridarpatil/bifrost/handlers"
	"github.com/shridarpatil/bifrost/store"
	"github.com/shridarpatil/bifrost/whatsapp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	waClient, err := whatsapp.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create WhatsApp client: %v", err)
	}

	mediaStore, err := store.NewMediaStore(cfg.Media.StorePath, cfg.Media.MaxSizeMB)
	if err != nil {
		log.Fatalf("Failed to create media store: %v", err)
	}

	eventProcessor := whatsapp.NewEventProcessor(cfg, waClient, mediaStore)
	waClient.SetEventHandler(eventProcessor)

	sessionMgr := whatsapp.NewSessionManager(waClient)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	authHandler := handlers.NewAuthHandler(sessionMgr, cfg)
	authHandler.RegisterRoutes(r)

	messageHandler := handlers.NewMessageHandler(waClient, mediaStore)
	messageHandler.RegisterRoutes(r)

	mediaHandler := handlers.NewMediaHandler(mediaStore, cfg)
	mediaHandler.RegisterRoutes(r)

	healthHandler := handlers.NewHealthHandler(sessionMgr)
	healthHandler.RegisterRoutes(r)

	sessionMgr.ConnectAll()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Starting bifrost on %s", addr)
	log.Printf("API Base URL: http://localhost%s", addr)
	if len(cfg.Instances) > 0 {
		log.Printf("Configured instances:")
		for phoneID, inst := range cfg.Instances {
			phone := inst.Phone
			if phone == "" {
				phone = phoneID
			}
			log.Printf("  - %s: http://localhost%s/auth/qr/%s", phone, addr, phoneID)
		}
	} else {
		log.Printf("QR Code URL: http://localhost%s/auth/qr/{phone_id}", addr)
	}

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	waClient.Close()
	log.Println("Goodbye!")
}
