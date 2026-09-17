package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"

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
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal("Failed to create migration driver:", err)
	}

	// Point to the migration files
	m, err := migrate.NewWithDatabaseInstance("file://./migrations", "postgres", driver)
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

	// Initialize AWS S3 Client
	log.Println("Initializing AWS Config...")
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal("Failed to load AWS config:", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	bucketName := os.Getenv("S3_BUCKET_NAME")
	if bucketName == "" {
		log.Fatal("S3_BUCKET_NAME environment variable is not set")
	}
	log.Println("Successfully initialized S3 Client")

	// Initialize repository
	repo := &repository.Repository{
		DB: db,
	}

	// Create the API Handler
	h := &api.Handler{
		Repo:             repo,
		S3Client:         s3Client,
		BucketName:       bucketName,
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
			patientGroup.GET("/prescriptions/:filename", h.DownloadPrescription)
		}

		// Doctor-only Routes
		doctorGroup := authGroup.Group("")
		doctorGroup.Use(api.RequireRole("doctor"))
		{
			doctorGroup.GET("/doctor/appointments", h.GetDoctorAppointments)
			doctorGroup.GET("/doctor/patients", h.GetDoctorPatients)
			doctorGroup.POST("/prescriptions", h.CreatePrescription)
			doctorGroup.GET("/doctor/prescriptions/:filename", h.DoctorDownloadPrescription)
			doctorGroup.GET("/doctor/patients/:id/appointments", h.GetPatientHistoryAppointments)
			doctorGroup.GET("/doctor/patients/:id/prescriptions", h.GetPatientHistoryPrescriptions)
			doctorGroup.PATCH("/appointments/:id", h.MarkAppointmentAsCompleted)
		}
	}

	// Run the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}
