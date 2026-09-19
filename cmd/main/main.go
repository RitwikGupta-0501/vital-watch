package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"net/http"
	"os/signal"
	"syscall"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

/*
========================================
=        Database Initialization       =
========================================
*/
func init_db() *sql.DB {

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

	// Open the database connection pool
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatal("Failed to open database connection:", err)
	}

	// Tune database connection pool settings (DB-03)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// --- NEW: Add a retry loop for db.Ping() ---
	var dbErr error
	for i := 0; i < 5; i++ { // Try 5 times
		err = db.Ping()
		if err == nil {
			// Success!
			log.Println("Successfully connected to database!")
			return db
		}
		dbErr = err
		log.Println("Failed to ping database, retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
	// If the loop finishes, we failed
	log.Fatal("Failed to ping database after retries:", dbErr)
	return nil
}

/*
========================================
=           Migrations Runner          =
========================================
*/
func run_migrations(db *sql.DB) {
	log.Println("Running database migrations...")
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
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

	// Initialize DB
	var db = init_db()
	defer db.Close()

	// Run DB migrations
	run_migrations(db)

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

	// Initialize repository (ARCH-02)
	repo := repository.New(db)

	// Create the API Handler
	h := &api.Handler{
		Repo:             repo,
		Storage:          storageProvider,
		JWTSecret:        jwtSecret,
		DoctorInviteCode: doctorInviteCode,
	}

	// Set up Gin Server
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

	// --- Protected Routes ---
	authGroup := r.Group("/api")
	authGroup.Use(api.AuthMiddleware(jwtSecret))
	{
		// Common Profile Route (Accessible to both Patients & Doctors)
		authGroup.GET("/profile", h.GetUserProfile)

		// Patient-only Routes
		patientGroup := authGroup.Group("")
		patientGroup.Use(api.RequireRole("patient"))
		{
			patientGroup.GET("/doctors", h.GetDoctors)
			patientGroup.GET("/patient/appointments", h.GetPatientAppointments)
			patientGroup.GET("/patient/prescriptions", h.GetPatientPrescriptions)
			patientGroup.POST("/appointments", h.CreateAppointment)
			patientGroup.GET("/prescriptions/:filename/download-url", h.DownloadPrescription)
			patientGroup.GET("/prescriptions/:filename", h.DownloadPrescription) // alias
		}

		// Doctor-only Routes
		doctorGroup := authGroup.Group("")
		doctorGroup.Use(api.RequireRole("doctor"))
		{
			doctorGroup.GET("/doctor/appointments", h.GetDoctorAppointments)
			doctorGroup.GET("/doctor/patients", h.GetDoctorPatients)
			doctorGroup.POST("/prescriptions/upload-url", h.GetPrescriptionUploadURL)
			doctorGroup.POST("/prescriptions", h.CreatePrescription)
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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server listen error: %v", err)
		}
	}()

	// Listen for OS signals for graceful shutdown (OPS-02)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received shutdown signal (%v). Draining in-flight connections...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully.")
}
