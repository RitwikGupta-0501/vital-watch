package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/ocr"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/queue"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

/*
========================================
=        Database Initialization       =
========================================
*/
func init_db(ctx context.Context) *pgxpool.Pool {
	dbHost := os.Getenv("DB_HOST")
	dbPortStr := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	sslMode := os.Getenv("DB_SSLMODE")

	dbPort, err := strconv.Atoi(dbPortStr)
	if err != nil {
		log.Fatal("Invalid DB_PORT:", err)
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, sslMode)

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatal("Failed to parse database pool configuration:", err)
	}

	// Tune database connection pool settings (DB-03)
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 5 * time.Minute
	poolConfig.MaxConnIdleTime = 2 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatal("Failed to initialize database connection pool:", err)
	}

	// Retry loop for pool.Ping()
	var dbErr error
	for i := 0; i < 5; i++ {
		err = pool.Ping(ctx)
		if err == nil {
			log.Println("Successfully connected to database pool!")
			return pool
		}
		dbErr = err
		log.Println("Failed to ping database, retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
	log.Fatal("Failed to ping database after retries:", dbErr)
	return nil
}

/*
========================================
=           Migrations Runner          =
========================================
*/
func run_migrations(pool *pgxpool.Pool) {
	log.Println("Running database migrations...")
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		log.Fatal("Failed to create migration driver:", err)
	}

	// Point to the migration files
	m, err := migrate.NewWithDatabaseInstance("file://./migrations", "pgx5", driver)
	if err != nil {
		log.Fatal("Failed to create migration instance:", err)
	}

	// Run the migrations "up"
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatal("Failed to run migrations:", err)
	}

	log.Println("Database migrations finished successfully.")
}

/*
========================================
=                Main                  =
========================================
*/
func main() {
	// Initialize environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	// Validate critical environment variables
	jwtSecretStr := os.Getenv("JWT_SECRET")
	if jwtSecretStr == "" {
		log.Fatal("FATAL: JWT_SECRET environment variable is not set")
	}
	jwtSecret := []byte(jwtSecretStr)
	doctorInviteCode := os.Getenv("DOCTOR_INVITE_CODE")
	if doctorInviteCode == "" {
		log.Fatal("FATAL: DOCTOR_INVITE_CODE environment variable is not set")
	}

	// Initialize DB Pool
	ctx := context.Background()
	pool := init_db(ctx)
	defer pool.Close()

	// Run DB migrations
	run_migrations(pool)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize Storage Provider (S3 or Local Disk)
	storageType := os.Getenv("STORAGE_PROVIDER")
	if storageType == "" {
		storageType = "local"
	}

	var storageProvider storage.Provider
	switch storageType {
	case "s3":
		log.Println("Initializing AWS Config for S3 Storage Provider...")
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Fatal("Failed to load AWS config:", err)
		}
		bucketName := os.Getenv("S3_BUCKET_NAME")
		if bucketName == "" {
			log.Fatal("S3_BUCKET_NAME environment variable is not set")
		}
		s3Client := s3.NewFromConfig(cfg)
		storageProvider = storage.NewS3Provider(s3Client, bucketName)
		log.Println("Successfully initialized S3 Storage Provider")

	default: // "local"
		log.Println("Initializing Local Disk Storage Provider...")
		localStorageURL := os.Getenv("LOCAL_STORAGE_BASE_URL")
		if localStorageURL == "" {
			localStorageURL = "http://localhost:" + port
		}
		localSecretStr := os.Getenv("LOCAL_STORAGE_SECRET")
		var localSecret []byte
		if localSecretStr != "" {
			localSecret = []byte(localSecretStr)
		} else {
			localSecret = jwtSecret
		}
		localProv, err := storage.NewLocalProvider("./storage", localStorageURL, localSecret)
		if err != nil {
			log.Fatal("Failed to initialize local storage:", err)
		}
		storageProvider = localProv
		log.Println("Successfully initialized Local Storage Provider (Base URL:", localStorageURL, ")")
	}

	// Initialize Multi-Provider Vision AI OCR Engine
	ocrManager := ocr.NewManagerFromEnv(ctx)
	if ocrManager.IsEnabled() {
		providersStr := os.Getenv("OCR_PROVIDERS")
		if providersStr == "" {
			providersStr = "gemini,claude,openai (default)"
		}
		log.Printf("Vision AI OCR enabled with active providers: %s", providersStr)
	} else {
		log.Println("Vision AI OCR is disabled (zero-config / no active provider keys). Prescriptions will enter needs_review for manual clinician entry.")
	}

	// Initialize Repository (breaks initialization cycle with River)
	repo := repository.New(pool, nil)

	// Initialize PDF Generator, DDI Safety Engine, and Real-Time SSE Broker
	pdfGen := pdf.NewStandardPDFGenerator()
	safetyChecker := safety.NewOpenFDAChecker()
	notifier := notifications.NewSSEBroker()

	// Initialize River Task Queue & Worker
	ocrWorker := queue.NewPrescriptionOCRWorker(repo, storageProvider, ocrManager)
	ocrWorker.SetNotifier(notifier)
	riverClient, err := queue.NewClient(pool, ocrWorker)
	if err != nil {
		log.Fatalf("Failed to initialize River task queue: %v", err)
	}
	repo.SetRiverClient(riverClient.RiverClient)

	// Start River Queue Consumer
	log.Println("Starting River task queue worker...")
	if err := riverClient.RiverClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start River background worker: %v", err)
	}

	// Create the API Handler
	h := &api.Handler{
		Repo:             repo,
		Storage:          storageProvider,
		JWTSecret:        jwtSecret,
		DoctorInviteCode: doctorInviteCode,
		OCREnabled:       ocrManager.IsEnabled(),
		PDFGenerator:     pdfGen,
		SafetyChecker:    safetyChecker,
		Notifier:         notifier,
	}

	// Set up Gin Router
	r := setupRouter(h, storageType, jwtSecret)

	// Set up HTTP Server with timeouts (OPS-02)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Run server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on port %s...", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server listen error: %v", err)
		}
	}()

	// Listen for OS signals for graceful shutdown (OPS-02)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received shutdown signal (%v). Draining in-flight connections and queue workers...", sig)

	// Graceful shutdown: Real-time notification broker (unblocks active SSE streams so HTTP server can drain)
	log.Println("Closing notification broker subscriptions...")
	notifier.Shutdown()

	// Graceful shutdown: HTTP Server (stop accepting new requests, finish in-flight requests)
	log.Println("Stopping HTTP server listener and draining in-flight requests...")
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer httpCancel()
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Graceful shutdown: River Task Queue (drains worker after in-flight requests finish enqueueing)
	log.Println("Stopping River queue consumer...")
	riverShutdownCtx, riverCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer riverCancel()
	if err := riverClient.RiverClient.Stop(riverShutdownCtx); err != nil {
		log.Printf("River client stop error: %v", err)
	}

	log.Println("Server exited gracefully.")
}

// setupRouter builds and configures the Gin engine and route tree
func setupRouter(h *api.Handler, storageType string, jwtSecret []byte) *gin.Engine {
	r := gin.Default()

	// Configure CORS
	corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	allowedOrigins := []string{"http://localhost:3000", "https://d11ox9eozk6am1.cloudfront.net"}
	if corsOrigins != "" {
		originsList := strings.Split(corsOrigins, ",")
		var cleaned []string
		for _, o := range originsList {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			allowedOrigins = cleaned
		}
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// -----------------------
	// -       Routes        -
	// -----------------------
	r.GET("/api/ping", h.Ping)
	r.POST("/api/register", h.Register)
	r.POST("/api/login", h.Login)

	// Real-Time Notification SSE Stream (Supports EventSource query token and Bearer header)
	r.GET("/api/notifications/stream", api.SSEAuthMiddleware(jwtSecret), h.StreamNotifications)

	// --- Protected Routes (Strict Bearer Header Authentication) ---
	authGroup := r.Group("/api")
	authGroup.Use(api.AuthMiddleware(jwtSecret))
	{
		// Common Profile & Prescription Routes (Accessible to Patients & Authorized Doctors)
		authGroup.GET("/profile", h.GetUserProfile)
		authGroup.GET("/prescriptions/:id", h.GetPrescriptionByID)
		authGroup.GET("/prescriptions/:id/download-url", h.DownloadPrescription)

		// Patient-only Routes
		patientGroup := authGroup.Group("")
		patientGroup.Use(api.RequireRole("patient"))
		{
			patientGroup.GET("/doctors", h.GetDoctors)
			patientGroup.GET("/patient/appointments", h.GetPatientAppointments)
			patientGroup.GET("/patient/prescriptions", h.GetPatientPrescriptions)
			patientGroup.POST("/appointments", h.CreateAppointment)
			patientGroup.GET("/patient/prescriptions/:filename/download-url", h.DownloadPrescription)
		}

		// Doctor-only Routes
		doctorGroup := authGroup.Group("")
		doctorGroup.Use(api.RequireRole("doctor"))
		{
			doctorGroup.GET("/doctor/appointments", h.GetDoctorAppointments)
			doctorGroup.GET("/doctor/patients", h.GetDoctorPatients)
			doctorGroup.POST("/prescriptions/upload-url", h.GetPrescriptionUploadURL)
			doctorGroup.POST("/prescriptions", h.CreatePrescription)
			doctorGroup.POST("/prescriptions/digital", h.CreateDigitalPrescription)
			doctorGroup.GET("/prescriptions/pending-review", h.GetPendingReviewPrescriptions)
			doctorGroup.PATCH("/prescriptions/:id/verify", h.VerifyPrescription)
			doctorGroup.GET("/doctor/prescriptions/:filename/download-url", h.DoctorDownloadPrescription)
			doctorGroup.GET("/doctor/prescriptions/:filename", h.DoctorDownloadPrescription) // alias
			doctorGroup.GET("/doctor/patients/:id/appointments", h.GetPatientHistoryAppointments)
			doctorGroup.GET("/doctor/patients/:id/prescriptions", h.GetPatientHistoryPrescriptions)
			doctorGroup.PATCH("/appointments/:id", h.MarkAppointmentAsCompleted)
		}
	}

	// Local development file routes with cryptographic HMAC pre-signing
	if storageType == "local" {
		r.PUT("/storage/upload", h.HandleLocalStorageUpload)
		r.GET("/storage/download", h.HandleLocalStorageDownload)
	}

	return r
}
