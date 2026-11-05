package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"linkdesk/database"
	"linkdesk/handler"
	"linkdesk/service"
)

var (
	port         = flag.String("port", env("PORT", "8080"), "HTTP server port")
	host         = flag.String("host", env("HOST", "127.0.0.1"), "HTTP listen host")
	dbPath       = flag.String("db", env("DB_PATH", "data/linkdesk.db"), "SQLite database path")
	baseURL      = flag.String("base-url", env("BASE_URL", "http://localhost:8080"), "Base URL for shortened URLs")
	cliMode      = flag.Bool("cli", false, "Run in CLI mode")
	templatesDir = flag.String("templates", "templates", "Templates directory")
	username     = flag.String("username", env("AUTH_USERNAME", ""), "Username for HTTP Basic Auth")
	password     = flag.String("password", env("AUTH_PASSWORD", ""), "Password for HTTP Basic Auth; prefer environment variable")
)

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	flag.Parse()

	if (*username == "") != (*password == "") {
		log.Fatal("--username and --password must both be provided together")
	}
	if !*cliMode && *host != "localhost" && *host != "127.0.0.1" && *host != "::1" && *username == "" {
		log.Fatal("Non-loopback listening requires AUTH_USERNAME and AUTH_PASSWORD")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0750); err != nil {
		log.Fatal(err)
	}

	db, err := database.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	urlService := service.New(db)

	if *cliMode {
		cliHandler := handler.NewCLIHandler(urlService, *baseURL)
		rootCmd := cliHandler.SetupCommands()
		rootCmd.SetArgs(flag.Args())
		if err := rootCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	httpHandler, err := handler.NewHTTPHandler(urlService, *baseURL, *templatesDir)
	if err != nil {
		log.Fatalf("Failed to create HTTP handler: %v", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)

	httpHandler.SetupRoutes(router, *username, *password)

	addr := net.JoinHostPort(*host, *port)
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("Starting server on %s", addr)
		log.Printf("URL shortener available at %s", *baseURL)
		if *username != "" {
			log.Printf("HTTP Basic Auth enabled")
		} else {
			log.Printf("WARNING: HTTP Basic Auth is disabled. Set --username and --password to protect the admin interface.")
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}
