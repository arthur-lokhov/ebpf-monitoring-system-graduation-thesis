package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/epbf-monitoring/epbf-monitor/internal/api"
	"github.com/epbf-monitoring/epbf-monitor/internal/filter"
	"github.com/epbf-monitoring/epbf-monitor/internal/logger"
	"github.com/epbf-monitoring/epbf-monitor/internal/metrics"
	"github.com/epbf-monitoring/epbf-monitor/internal/plugin"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/postgres"
	"github.com/epbf-monitoring/epbf-monitor/internal/storage/s3"
)

func main() {
	// Initialize logger
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	if err := logger.Init(logLevel); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("🚀 Starting epbf-monitoring...", "version", "0.1.0")

	// Load .env file
	loadEnvFile()

	// Create context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize database
	logger.Info("📦 Connecting to PostgreSQL...")
	dbConfig := postgres.DefaultConfig()

	// Override with env vars if present
	if host := getEnv("DB_HOST", ""); host != "" {
		dbConfig.Host = host
		logger.Debug("DB_HOST overridden", "host", host)
	}
	if port := getEnv("DB_PORT", ""); port != "" {
		fmt.Sscanf(port, "%d", &dbConfig.Port)
		logger.Debug("DB_PORT overridden", "port", port)
	}
	if user := getEnv("DB_USER", ""); user != "" {
		dbConfig.User = user
		logger.Debug("DB_USER overridden", "user", user)
	}
	if password := getEnv("DB_PASSWORD", ""); password != "" {
		dbConfig.Password = password
		logger.Debug("DB_PASSWORD overridden")
	}
	if database := getEnv("DB_NAME", ""); database != "" {
		dbConfig.Database = database
		logger.Debug("DB_NAME overridden", "database", database)
	}

	logger.Debug("PostgreSQL config",
		"host", dbConfig.Host,
		"port", dbConfig.Port,
		"user", dbConfig.User,
		"database", dbConfig.Database)

	db, err := postgres.NewClient(dbConfig)
	if err != nil {
		logger.Warn("⚠️  Database connection failed", "error", err.Error())
		logger.Info("Running without database - some features will be disabled")
		db = nil
	} else {
		logger.Info("✅ Connected to PostgreSQL", "host", dbConfig.Host, "database", dbConfig.Database)
		defer db.Close()
	}

	// Initialize S3 storage
	logger.Info("📦 Connecting to S3...")
	s3Config := s3.DefaultConfig()

	if endpoint := getEnv("S3_ENDPOINT", ""); endpoint != "" {
		s3Config.Endpoint = endpoint
		logger.Debug("S3_ENDPOINT overridden", "endpoint", endpoint)
	}
	if accessKey := getEnv("S3_ACCESS_KEY", ""); accessKey != "" {
		s3Config.AccessKey = accessKey
		logger.Debug("S3_ACCESS_KEY overridden")
	}
	if secretKey := getEnv("S3_SECRET_KEY", ""); secretKey != "" {
		s3Config.SecretKey = secretKey
		logger.Debug("S3_SECRET_KEY overridden")
	}
	if bucket := getEnv("S3_BUCKET", ""); bucket != "" {
		s3Config.BucketName = bucket
		logger.Debug("S3_BUCKET overridden", "bucket", bucket)
	}
	if region := getEnv("S3_REGION", ""); region != "" {
		s3Config.Region = region
		logger.Debug("S3_REGION overridden", "region", region)
	}

	logger.Debug("S3 config",
		"endpoint", s3Config.Endpoint,
		"region", s3Config.Region,
		"bucket", s3Config.BucketName,
		"access_key_set", s3Config.AccessKey != "")

	s3Client, err := s3.NewClient(s3Config)
	if err != nil {
		logger.Warn("⚠️  S3 connection failed", "error", err.Error())
		logger.Info("Running without object storage - some features will be disabled")
		s3Client = nil
	} else {
		// Test S3 connection
		if err := s3Client.Health(ctx); err != nil {
			logger.Warn("⚠️  S3 health check failed", "error", err.Error())
			logger.Info("S3 may not be accessible - uploads will fail")
		} else {
			logger.Info("✅ Connected to S3", "endpoint", s3Config.Endpoint, "bucket", s3Config.BucketName)
		}
	}

	// Initialize repositories
	var pluginRepo *postgres.PluginRepo
	var pluginStorage *s3.PluginStorage
	var filterRepo *postgres.FilterRepo
	var dashboardRepo *postgres.DashboardRepo
	var metricRepo *postgres.MetricRepo

	if db != nil {
		pluginRepo = postgres.NewPluginRepo(db)
		filterRepo = postgres.NewFilterRepo(db)
		dashboardRepo = postgres.NewDashboardRepo(db)
		metricRepo = postgres.NewMetricRepo(db)
		logger.Info("✅ All repositories initialized")
	}

	if s3Client != nil {
		pluginStorage = s3.NewPluginStorage(s3Client)
		logger.Info("✅ Plugin storage initialized")
	}

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector()
	logger.Info("✅ Metrics collector initialized")

	// Initialize filter engine
	filterEngine := filter.NewEngine()
	filterEngine.StartCleanupRoutine(ctx)
	logger.Info("✅ Filter engine initialized")

	// Initialize metrics service
	metricsService := metrics.NewService(metricsCollector, filterEngine)
	logger.Info("✅ Metrics service initialized")

	// Initialize plugin service
	var pluginService *plugin.Service
	if pluginRepo != nil && pluginStorage != nil {
		buildDir := getEnv("BUILD_DIR", "/tmp/epbf-builds")
		builderImage := getEnv("BUILDER_IMAGE", "epbf-monitor-builder:latest")
		enableDocker := getEnv("ENABLE_DOCKER", "true") != "false"

		logger.Info("🔨 Initializing plugin service",
			"build_dir", buildDir,
			"builder_image", builderImage,
			"enable_docker", enableDocker)

		var err error
		pluginService, err = plugin.NewService(
			pluginRepo,
			pluginStorage,
			metricsCollector,
			filterEngine,
			plugin.Config{
				BuildDir:     buildDir,
				BuilderImage: builderImage,
				EnableDocker: enableDocker,
			},
		)
		if err != nil {
			logger.Error("⚠️  Plugin service initialization failed", "error", err.Error())
		} else {
			logger.Info("✅ Plugin service initialized")

			// Start all plugins from database
			if err := pluginService.StartAllPlugins(ctx); err != nil {
				logger.Warn("⚠️  Failed to start some plugins", "error", err.Error())
			}
		}
	} else {
		logger.Warn("⚠️  Plugin service not initialized - missing dependencies",
			"has_db", pluginRepo != nil,
			"has_s3", pluginStorage != nil)
	}

	// Initialize API handlers
	handlers := api.NewHandlers()
	handlers.SetMetrics(metricsCollector)
	handlers.SetMetricsService(metricsService)
	handlers.SetPluginService(pluginService)
	handlers.SetFilterEngine(filterEngine)
	handlers.SetFilterRepo(filterRepo)
	handlers.SetDashboardRepo(dashboardRepo)
	handlers.SetMetricRepo(metricRepo)
	logger.Info("🔌 API handlers initialized")

	// Setup router
	router := api.NewRouter()
	api.SetupRoutes(router, handlers)
	logger.Info("🛣️  HTTP router configured")

	// Start server
	port := getEnv("PORT", "8080")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("🌐 Starting HTTP server", "port", port)
	logger.Info("📊 Endpoints:",
		"health", fmt.Sprintf("http://localhost:%s/health", port),
		"metrics", fmt.Sprintf("http://localhost:%s/metrics", port),
		"api", fmt.Sprintf("http://localhost:%s/api/v1", port),
		"ws", fmt.Sprintf("ws://localhost:%s/ws", port))

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("❌ Server failed", "error", err.Error())
		}
	}()

	logger.Info("✅ Server is ready to accept connections")

	// Wait for interrupt signal
	<-ctx.Done()

	logger.Info("\n🛑 Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("❌ Server shutdown failed", "error", err.Error())
	}

	logger.Info("✅ Server stopped")
	fmt.Println("👋 Bye!")
}

// getEnv returns environment variable or default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loadEnvFile loads environment variables from .env files
// It tries multiple locations in order:
// 1. .env in current directory
// 2. deployments/.env relative to current directory
// 3. ../deployments/.env (when running from cmd/epbf-monitor)
func loadEnvFile() {
	// Try multiple possible locations
	envPaths := []string{
		".env",
		"deployments/.env",
		"../deployments/.env",
	}

	loaded := false
	for _, envPath := range envPaths {
		// Convert to absolute path for better logging
		absPath, err := filepath.Abs(envPath)
		if err != nil {
			absPath = envPath
		}

		if err := godotenv.Load(envPath); err == nil {
			logger.Info("✅ Loaded environment variables", "file", absPath)
			loaded = true
			break
		}
	}

	if !loaded {
		logger.Debug("No .env file found, using system environment variables")
	}
}
