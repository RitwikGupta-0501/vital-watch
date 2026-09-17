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
	DoctorName      string    `json:"doctor_name,omitempty"`
	DoctorSpecialty string    `json:"doctor_specialty,omitempty"`
	PatientName     string    `json:"patient_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Prescription struct {
	ID         uuid.UUID `json:"id"`
	PatientID  uuid.UUID `json:"patient_id"`
	DoctorID   uuid.UUID `json:"doctor_id"`
	Medication string    `json:"medication"`
	Notes      string    `json:"notes"`
	FileName   string    `json:"file_name"`
	CreatedAt  time.Time `json:"created_at"`
	DoctorName string    `json:"doctor_name,omitempty"`
}
