package models

import (
	"time"

	"github.com/google/uuid"
)

type Authenticatable interface {
	GetID() uuid.UUID
	GetHashedPassword() string
	GetRole() string
}

type User struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"-"`
	Role           string    `json:"role"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (u User) GetID() uuid.UUID {
	return u.ID
}

func (u User) GetHashedPassword() string {
	return u.HashedPassword
}

func (u User) GetRole() string {
	return u.Role
}

type Patient struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	HashedPassword string    `json:"-"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}

func (p Patient) GetID() uuid.UUID {
	return p.ID
}

func (p Patient) GetHashedPassword() string {
	return p.HashedPassword
}

func (p Patient) GetRole() string {
	return "patient"
}

type Doctor struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	HashedPassword string    `json:"-"`
	Role           string    `json:"role"`
	Specialty      string    `json:"specialty"`
	Experience     int       `json:"experience"`
	Available      bool      `json:"available"`
	CreatedAt      time.Time `json:"created_at"`
}

func (d Doctor) GetID() uuid.UUID {
	return d.ID
}

func (d Doctor) GetHashedPassword() string {
	return d.HashedPassword
}

func (d Doctor) GetRole() string {
	return "doctor"
}

type Appointment struct {
	ID              uuid.UUID `json:"id"`
	DoctorID        uuid.UUID `json:"doctor_id"`
	PatientID       uuid.UUID `json:"patient_id"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Status          string    `json:"status"`
	Type            string    `json:"type"`
	MeetingLink     string    `json:"meeting_link,omitempty"`
	MeetingID       string    `json:"meeting_id,omitempty"`
	DoctorName      string    `json:"doctor_name,omitempty"`
	DoctorSpecialty string    `json:"doctor_specialty,omitempty"`
	PatientName     string    `json:"patient_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type PrescriptionItem struct {
	ID             uuid.UUID `json:"id"`
	PrescriptionID uuid.UUID `json:"prescription_id"`
	MedicationName string    `json:"medication_name"`
	Dosage         string    `json:"dosage,omitempty"`
	Frequency      string    `json:"frequency,omitempty"`
	Duration       string    `json:"duration,omitempty"`
	Timing         string    `json:"timing,omitempty"`
	Instructions   string    `json:"instructions,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Prescription struct {
	ID          uuid.UUID          `json:"id"`
	PatientID   uuid.UUID          `json:"patient_id"`
	DoctorID    uuid.UUID          `json:"doctor_id"`
	Source      string             `json:"source"`
	Status      string             `json:"status"`
	FileName    string             `json:"file_name,omitempty"`
	Notes       string             `json:"notes"`
	OCRProvider string             `json:"ocr_provider,omitempty"`
	Items       []PrescriptionItem `json:"items"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	DoctorName  string             `json:"doctor_name,omitempty"`
	PatientName string             `json:"patient_name,omitempty"`
}

type DoctorSchedule struct {
	ID           uuid.UUID `json:"id"`
	DoctorID     uuid.UUID `json:"doctor_id"`
	DayOfWeek    int       `json:"day_of_week"` // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
	StartTime    string    `json:"start_time"`  // "HH:MM" e.g. "09:00"
	EndTime      string    `json:"end_time"`    // "HH:MM" e.g. "17:00"
	SlotDuration int       `json:"slot_duration"`
	Timezone     string    `json:"timezone"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TimeSlot struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Available bool      `json:"available"`
}

type PatientVital struct {
	ID               uuid.UUID `json:"id"`
	PatientID        uuid.UUID `json:"patient_id"`
	RecordedBy       uuid.UUID `json:"recorded_by"`
	RecordedAt       time.Time `json:"recorded_at"`
	SystolicBP       *int      `json:"systolic_bp,omitempty"`
	DiastolicBP      *int      `json:"diastolic_bp,omitempty"`
	HeartRate        *int      `json:"heart_rate,omitempty"`
	BloodGlucose     *float64  `json:"blood_glucose,omitempty"`
	OxygenSaturation *float64  `json:"oxygen_saturation,omitempty"`
	Temperature      *float64  `json:"temperature,omitempty"`
	WeightKg         *float64  `json:"weight_kg,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	RecorderName     string    `json:"recorder_name,omitempty"`
	RecorderRole     string    `json:"recorder_role,omitempty"`
}

type MedicationLog struct {
	ID                 uuid.UUID  `json:"id"`
	PatientID          uuid.UUID  `json:"patient_id"`
	PrescriptionItemID uuid.UUID  `json:"prescription_item_id"`
	ScheduledDate      time.Time  `json:"scheduled_date"`
	TimeOfDay          string     `json:"time_of_day"` // 'morning', 'afternoon', 'evening', 'bedtime', 'as_needed'
	DoseNumber         int        `json:"dose_number"`
	MealTiming         string     `json:"meal_timing,omitempty"`
	Status             string     `json:"status"` // 'pending', 'taken', 'skipped'
	TakenAt            *time.Time `json:"taken_at,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	MedicationName     string     `json:"medication_name,omitempty"`
	Dosage             string     `json:"dosage,omitempty"`
	Timing             string     `json:"timing,omitempty"`
	Instructions       string     `json:"instructions,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type RefreshToken struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	TokenHash         string     `json:"-"`
	ExpiresAt         time.Time  `json:"expires_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	ReplacedByTokenID *uuid.UUID `json:"replaced_by_token_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type PhiAuditLog struct {
	ID           uuid.UUID  `json:"id"`
	UserID       *uuid.UUID `json:"user_id,omitempty"`
	UserRole     string     `json:"user_role,omitempty"`
	Action       string     `json:"action"`
	ResourceType string     `json:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id,omitempty"`
	PatientID    *uuid.UUID `json:"patient_id,omitempty"`
	IPAddress    string     `json:"ip_address,omitempty"`
	UserAgent    string     `json:"user_agent,omitempty"`
	RequestID    *uuid.UUID `json:"request_id,omitempty"`
	StatusCode   int        `json:"status_code"`
	Metadata     string     `json:"metadata,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

