package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/crypto"
	"github.com/RitwikGupta-0501/vital-watch/internal/logger"
	"github.com/RitwikGupta-0501/vital-watch/internal/metrics"
	"github.com/RitwikGupta-0501/vital-watch/internal/middleware"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/ocr"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/queue"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/internal/telehealth"
)

/*
========================================
=        Database Initialization       =
========================================
*/

// buildDatabaseDSN constructs a connection string for pgxpool.ParseConfig.
// Priority: DATABASE_URL env var (matches CI and 12-factor cloud deployments),
// then individual DB_* vars assembled into a properly URL-encoded URL so that
// passwords containing spaces, quotes, or backslashes are handled correctly.
func buildDatabaseDSN() string {
	if u := os.Getenv("DATABASE_URL"); u != "" {
		return u
	}

	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	sslMode := os.Getenv("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(dbUser, dbPassword),
		Host:   fmt.Sprintf("%s:%s", dbHost, dbPort),
		Path:   dbName,
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	return u.String()
}

func init_db(ctx context.Context) *pgxpool.Pool {
	connStr := buildDatabaseDSN()

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatal("Failed to parse database pool configuration:", err)
	}

	// Pool sizing budget:
	//   HTTP REST layer      ~40 (burst)
	//   River worker pool    ≤12 (10 workers + 2 River internal)
	//   HIPAA Async Auditor   5  (DefaultWorkerCount)
	//   SSE pg_notify LISTEN  1  (permanent)
	//   Safety buffer        17
	//   ─────────────────────────
	//   Total                75  (override via DB_MAX_CONNS)
	maxConns := 75
	if val := os.Getenv("DB_MAX_CONNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			maxConns = n
		}
	}
	minConns := 10
	if val := os.Getenv("DB_MIN_CONNS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			minConns = n
		}
	}

	poolConfig.MaxConns = int32(maxConns)
	poolConfig.MinConns = int32(minConns)
	poolConfig.MaxConnLifetime = 1 * time.Hour
	// Jitter desynchronises mass connection recycling that would otherwise
	// produce a thundering-herd TCP reconnect storm every 60 minutes.
	poolConfig.MaxConnLifetimeJitter = 5 * time.Minute
	poolConfig.MaxConnIdleTime = 15 * time.Minute
	// Actively probe idle connections so silent dead sockets (cloud NAT/firewall
	// idle-timeout drops) are evicted before a request attempts to use them.
	poolConfig.HealthCheckPeriod = 30 * time.Second
	// Bound the TCP handshake so a hung PostgreSQL cannot stall startup indefinitely.
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second

	// Per-session server-side safety guards:
	//   application_name  – identifies this service in pg_stat_activity / pg_locks
	//   statement_timeout – aborts runaway queries before they hold row-level locks
	//   idle_in_transaction_session_timeout – evicts stalled transactions that block VACUUM/DDL
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "vital-watch-backend"
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = "30000"                   // 30 s
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "10000" // 10 s

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatal("Failed to initialize database connection pool:", err)
	}

	// Retry loop: each attempt is individually time-bounded so a partially
	// reachable PostgreSQL cannot block startup longer than 5 × 3 s + 10 s backoff.
	backoff := 500 * time.Millisecond
	var lastErr error
	for i := 1; i <= 5; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		lastErr = pool.Ping(pingCtx)
		cancel()
		if lastErr == nil {
			slog.Info("Successfully connected to database pool",
				"max_conns", maxConns,
				"min_conns", minConns,
			)
			// Register Prometheus DB pool metrics collector
			metrics.Register(pool)
			return pool
		}
		slog.Warn("Database ping failed, retrying...", "attempt", i, "error", lastErr)
		time.Sleep(backoff)
		backoff *= 2
	}
	log.Fatal("Failed to connect to database after retries:", lastErr)
	return nil
}

/*
========================================
=           Migrations Runner          =
========================================
*/
func run_migrations(pool *pgxpool.Pool) {
	// AUTO_MIGRATE=false lets multi-replica deployments skip in-process DDL
	// and rely on a dedicated migration job / initContainer instead.
	if os.Getenv("AUTO_MIGRATE") == "false" {
		slog.Info("AUTO_MIGRATE=false; skipping in-process migration runner")
		return
	}

	slog.Info("Running database migrations...")
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
	// Release the file-system source reader and the database driver reference.
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Warn("Migration source close notice", "error", srcErr)
		}
		if dbErr != nil {
			slog.Warn("Migration database driver close notice", "error", dbErr)
		}
	}()

	// Run the migrations "up"
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatal("Failed to run migrations:", err)
	}

	slog.Info("Database migrations finished successfully.")
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
		slog.Info("No .env file found, relying on system environment variables")
	}

	// Re-initialize logger to apply LOG_LEVEL and LOG_FORMAT loaded from .env
	logger.DefaultLogger = logger.InitLogger()
	slog.SetDefault(logger.DefaultLogger)

	// Validate critical environment variables
	jwtSecretStr := os.Getenv("JWT_SECRET")
	if jwtSecretStr == "" {
		log.Fatal("FATAL: JWT_SECRET environment variable is not set")
	}
	if len(jwtSecretStr) < 32 {
		log.Fatal("FATAL: JWT_SECRET must contain at least 32 bytes (256 bits) of entropy")
	}
	jwtSecret := []byte(jwtSecretStr)
	doctorInviteCode := os.Getenv("DOCTOR_INVITE_CODE")
	adminInviteCode := os.Getenv("ADMIN_INVITE_CODE")
	if doctorInviteCode == "" {
		log.Fatal("FATAL: DOCTOR_INVITE_CODE environment variable is not set")
	}
	if len(doctorInviteCode) < 12 {
		log.Fatal("FATAL: DOCTOR_INVITE_CODE must contain at least 12 characters of entropy")
	}
	if adminInviteCode == "" {
		log.Fatal("FATAL: ADMIN_INVITE_CODE environment variable is not set")
	}
	if len(adminInviteCode) < 12 {
		log.Fatal("FATAL: ADMIN_INVITE_CODE must contain at least 12 characters of entropy")
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
		slog.Info("Initializing AWS Config for S3 Storage Provider...")
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
		slog.Info("Successfully initialized S3 Storage Provider")

	default: // "local"
		slog.Info("Initializing Local Disk Storage Provider...")
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
		slog.Info("Successfully initialized Local Storage Provider", "base_url", localStorageURL)
	}



	// Initialize Repository (breaks initialization cycle with River)
	repo := repository.New(pool, nil)

	// Initialize Cryptography Service
	cipherService, err := crypto.NewCipherService(os.Getenv("KMS_MASTER_KEY"))
	if err != nil {
		log.Fatalf("Failed to initialize CipherService: %v", err)
	}

	// Initialize Multi-Provider Vision AI OCR Engine
	ocrManager := ocr.NewManager(repo, cipherService)
	if ocrManager.IsEnabled() {
		slog.Info("Vision AI OCR enabled (BYOK Tenant Mode)")
	} else {
		slog.Info("Vision AI OCR is disabled (zero-config / no active provider keys). Prescriptions will enter needs_review for manual clinician entry.")
	}

	// Initialize PDF Generator, DDI Safety Engine, Real-Time SSE Broker, and Telehealth
	pdfGen := pdf.NewStandardPDFGenerator()
	safetyChecker := safety.NewOpenFDAChecker()
	notifier := notifications.NewSSEBroker(pool)
	telehealthProv := telehealth.NewTelehealthManager(jwtSecret)
	dlqFilePath := os.Getenv("AUDIT_DLQ_PATH")
	if dlqFilePath == "" {
		dlqFilePath = "audit_dlq.jsonl"
	}
	dlq := audit.NewFileDLQ(dlqFilePath)
	auditor := audit.NewAsyncAuditor(repo, dlq, 1000)

	// Initialize River Task Queue & Worker
	ocrWorker := queue.NewPrescriptionOCRWorker(repo, storageProvider, ocrManager)
	ocrWorker.SetNotifier(notifier)
	riverClient, err := queue.NewClient(pool, ocrWorker)
	if err != nil {
		log.Fatalf("Failed to initialize River task queue: %v", err)
	}
	repo.SetRiverClient(riverClient.RiverClient)

	// Start River Queue Consumer
	slog.Info("Starting River task queue worker...")
	if err := riverClient.RiverClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start River background worker: %v", err)
	}

	// Create the API Handler
	h := &api.Handler{
		Repo:             repo,
		Storage:          storageProvider,
		JWTSecret:        jwtSecret,
		DoctorInviteCode: doctorInviteCode,
		AdminInviteCode:  adminInviteCode,
		OCREnabled:       ocrManager.IsEnabled(),
		PDFGenerator:     pdfGen,
		SafetyChecker:    safetyChecker,
		Notifier:         notifier,
		Telehealth:       telehealthProv,
		Auditor:          auditor,
		CipherService:    cipherService,
		Pool:             pool,
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

	// Run server in a goroutine. Errors are funnelled back to main so the
	// graceful shutdown sequence always executes (pool.Close, auditor.Shutdown,
	// riverClient.Stop). Using log.Fatalf here would call os.Exit and bypass all
	// deferred functions, risking HIPAA audit data loss.
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Starting HTTP server", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Block until an OS signal or an unexpected server failure is received.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("Received shutdown signal. Draining in-flight connections and queue workers...", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("HTTP server failed unexpectedly; initiating emergency graceful shutdown", "error", err)
	}

	// Graceful shutdown: Real-time notification broker (unblocks active SSE streams so HTTP server can drain)
	slog.Info("Closing notification broker subscriptions...")
	notifier.Shutdown()

	// Graceful shutdown: HTTP Server (stop accepting new requests, finish in-flight requests)
	slog.Info("Stopping HTTP server listener and draining in-flight requests...")
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer httpCancel()
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	// Graceful shutdown: River Task Queue (drains worker after in-flight requests finish enqueueing)
	slog.Info("Stopping River queue consumer...")
	riverShutdownCtx, riverCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer riverCancel()
	if err := riverClient.RiverClient.Stop(riverShutdownCtx); err != nil {
		slog.Error("River client stop error", "error", err)
	}

	// Graceful shutdown: HIPAA ePHI Auditor
	slog.Info("Flushing and shutting down HIPAA audit worker...")
	auditShutdownCtx, auditCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer auditCancel()
	if err := auditor.Shutdown(auditShutdownCtx); err != nil {
		slog.Error("Auditor shutdown error", "error", err)
	}

	slog.Info("Server exited gracefully.")
}

// setupRouter builds and configures the Gin engine and route tree
func setupRouter(h *api.Handler, storageType string, jwtSecret []byte) *gin.Engine {
	if len(h.JWTSecret) == 0 && len(jwtSecret) > 0 {
		h.JWTSecret = jwtSecret
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestIDMiddleware())
	
	// HTTP Metrics Middleware
	metricsMiddleware := middleware.NewMetricsMiddleware()
	r.Use(metricsMiddleware.Middleware())

	r.Use(middleware.SecurityHeadersMiddleware())

	// Rate Limiting: 120 req/min general, 10 req/min for authentication endpoints
	generalLimiter := middleware.NewRateLimiter(120, time.Minute, h.Auditor)
	authLimiter := middleware.NewRateLimiter(10, time.Minute, h.Auditor)
	r.Use(generalLimiter.Middleware())

	// Configure CORS
	corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:5173", "https://d11ox9eozk6am1.cloudfront.net"}
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
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
	}))

	// -----------------------
	// -       Routes        -
	// -----------------------
	scrapeToken := os.Getenv("METRICS_SCRAPE_TOKEN")
	r.GET("/metrics", api.MetricsProtectionMiddleware(jwtSecret, scrapeToken), gin.WrapH(promhttp.Handler())) // Prometheus metrics endpoint
	r.GET("/healthz", h.HealthCheck)
	r.GET("/api/healthz", h.HealthCheck)
	r.GET("/api/ping", h.Ping)
	r.POST("/api/register", authLimiter.Middleware(), h.Register)
	r.POST("/api/login", authLimiter.Middleware(), h.Login)
	r.POST("/api/auth/refresh", authLimiter.Middleware(), h.RefreshToken)
	r.POST("/api/auth/logout", authLimiter.Middleware(), h.AuthMiddleware(), h.Logout)

	// Real-Time Notification SSE Stream (Supports EventSource query token and Bearer header)
	r.GET("/api/notifications/stream", h.SSEAuthMiddleware(), h.StreamNotifications)

	// --- Protected Routes (Strict Bearer Header Authentication) ---
	authGroup := r.Group("/api")
	authGroup.Use(middleware.SensitiveCacheControlMiddleware())
	authGroup.Use(h.AuthMiddleware())
	{
		// Common Profile & Prescription Routes (Accessible to Patients & Authorized Doctors)
		authGroup.GET("/profile", h.GetUserProfile)
		authGroup.GET("/prescriptions/:id", h.GetPrescriptionByID)
		authGroup.GET("/prescriptions/:id/download-url", h.DownloadPrescription)
		authGroup.GET("/compliance/audit-logs", h.GetComplianceAuditLogs)

		// Phase 4: Common Telehealth, Scheduling, Vitals, and Schedules
		authGroup.GET("/appointments/:id/meeting-room", h.GetAppointmentMeetingRoom)
		authGroup.PATCH("/appointments/:id/cancel", h.CancelAppointment)
		authGroup.GET("/doctors/:id/available-slots", h.GetDoctorAvailableSlots)
		authGroup.POST("/vitals", h.CreatePatientVital)
		authGroup.GET("/vitals", h.GetPatientVitals)
		authGroup.GET("/patients/medication-schedule", h.GetPatientMedicationSchedule)

		// Patient-only Routes
		patientGroup := authGroup.Group("")
		patientGroup.Use(api.RequireRole("patient"))
		{
			patientGroup.GET("/doctors", h.GetDoctors)
			patientGroup.GET("/patient/appointments", h.GetPatientAppointments)
			patientGroup.GET("/patient/prescriptions", h.GetPatientPrescriptions)
			patientGroup.POST("/appointments", h.CreateAppointment)
			patientGroup.GET("/patient/prescriptions/:filename/download-url", h.DownloadPrescription)
			patientGroup.POST("/patients/medication-schedule/log", h.LogMedicationAdherence)
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
			doctorGroup.PUT("/doctor/schedules", h.UpsertDoctorSchedule)
			doctorGroup.GET("/doctor/schedules", h.GetDoctorSchedules)
			doctorGroup.DELETE("/doctor/schedules/:day", h.DeleteDoctorSchedule)
		}

		// Admin-only Routes (accessible by tenant_admin and platform_admin)
		adminGroup := authGroup.Group("/admin")
		adminGroup.Use(api.RequireRole("admin", "tenant_admin", "platform_admin"))
		{
			adminGroup.GET("/users", h.ListUsers)
			adminGroup.PATCH("/users/:id/status", h.ToggleUserStatus)
			adminGroup.GET("/audit-logs", h.GetComplianceAuditLogs)
			adminGroup.GET("/ocr-settings", h.GetOCRSettings)
			adminGroup.POST("/ocr-settings/configs", h.CreateOCRConfig)
			adminGroup.PUT("/ocr-settings/chain", h.UpdateOCRChain)
		}

		// Platform-only Routes (accessible by platform_admin)
		platformGroup := authGroup.Group("/platform")
		platformGroup.Use(api.RequirePlatformAdmin())
		{
			platformGroup.POST("/tenants", h.CreateTenant)
			platformGroup.POST("/tenants/:id/invites", h.CreateTenantInvite)
		}
	}

	// Public Clinical Prescription Verification (scanned via QR code on prescription PDFs)
	r.GET("/verify/rx/:id", authLimiter.Middleware(), h.GetPrescriptionByID)

	// Local development file routes with cryptographic HMAC pre-signing
	if storageType == "local" {
		r.PUT("/storage/upload", h.HandleLocalStorageUpload)
		r.GET("/storage/download", h.HandleLocalStorageDownload)
	}

	return r
}
