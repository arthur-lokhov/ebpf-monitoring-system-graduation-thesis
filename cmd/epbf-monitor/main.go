package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/epbf-monitoring/epbf-monitor/internal/api"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/s3"
)

func main() {
	log.Println("🚀 Starting epbf-monitoring...")

	// Create context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize database
	log.Println("📦 Connecting to PostgreSQL...")
	dbConfig := postgres.DefaultConfig()
	
	// Override with env vars if present
	if host := os.Getenv("DB_HOST"); host != "" {
		dbConfig.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &dbConfig.Port)
	}
	if user := os.Getenv("DB_USER"); user != "" {
		dbConfig.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		dbConfig.Password = password
	}
	if database := os.Getenv("DB_NAME"); database != "" {
		dbConfig.Database = database
	}

	db, err := postgres.NewClient(dbConfig)
	if err != nil {
		log.Printf("⚠️  Database connection failed: %v (running without database)", err)
		db = nil
	} else {
		log.Println("✅ Connected to PostgreSQL")
		defer db.Close()
	}

	// Initialize S3 storage
	log.Println("📦 Connecting to S3...")
	s3Config := s3.DefaultConfig()
	
	if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
		s3Config.Endpoint = endpoint
	}
	if accessKey := os.Getenv("S3_ACCESS_KEY"); accessKey != "" {
		s3Config.AccessKey = accessKey
	}
	if secretKey := os.Getenv("S3_SECRET_KEY"); secretKey != "" {
		s3Config.SecretKey = secretKey
	}
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		s3Config.BucketName = bucket
	}

	s3Client, err := s3.NewClient(s3Config)
	if err != nil {
		log.Printf("⚠️  S3 connection failed: %v (running without object storage)", err)
		s3Client = nil
	} else {
		log.Println("✅ Connected to S3")
	}

	// TODO: Initialize services
	// - Plugin service
	// - Metrics service
	// - Filter service
	// - WASM runtime
	// - eBPF loader

	// Initialize API handlers
	handlers := api.NewHandlers()

	// Setup router
	router := api.NewRouter()
	api.SetupRoutes(router, handlers)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🌐 Server starting on :%s", port)
		log.Printf("📊 Health: http://localhost:%s/health", port)
		log.Printf("📈 Metrics: http://localhost:%s/metrics", port)
		log.Printf("🔌 API: http://localhost:%s/api/v1", port)
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()

	log.Println("\n🛑 Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("❌ Server shutdown failed: %v", err)
	}

	log.Println("✅ Server stopped")
	fmt.Println("👋 Bye!")
}
