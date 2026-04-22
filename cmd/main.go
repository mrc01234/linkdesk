package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mstgnz/self-hosted-url-shortener/database"
	"github.com/mstgnz/self-hosted-url-shortener/handler"
	"github.com/mstgnz/self-hosted-url-shortener/service"
)

var (
	port         = flag.String("port", "8080", "HTTP server port")
	dbPath       = flag.String("db", "data.db", "SQLite database path")
	baseURL      = flag.String("base-url", "http://localhost:8080", "Base URL for shortened URLs")
	cliMode      = flag.Bool("cli", false, "Run in CLI mode")
	templatesDir = flag.String("templates", "templates", "Templates directory")
	username     = flag.String("username", "", "Username for HTTP Basic Auth (requires --password)")
	password     = flag.String("password", "", "Password for HTTP Basic Auth (requires --username)")
)

func main() {
	flag.Parse()

	if (*username == "") != (*password == "") {
		log.Fatal("--username and --password must both be provided together")
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
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	httpHandler.SetupRoutes(router, *username, *password)

	addr := fmt.Sprintf(":%s", *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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
