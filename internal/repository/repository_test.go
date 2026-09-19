package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRepositoryInterface_MockImplementation(t *testing.T) {
	var repo Repository = &MockRepository{}
	if repo == nil {
		t.Fatalf("expected non-nil repository")
	}

	ctx := context.Background()
	dummyID := uuid.New()

	// Default fallback return values
	if patients, err := repo.GetPatientsByDoctorID(ctx, dummyID, 20, 0); err != nil || len(patients) != 0 {
		t.Fatalf("expected empty patients slice, got: %v, err: %v", patients, err)
	}

	if doctors, err := repo.GetDoctors(ctx, 20, 0); err != nil || len(doctors) != 0 {
		t.Fatalf("expected empty doctors slice, got: %v, err: %v", doctors, err)
	}

	if appts, err := repo.GetAppointmentsByDoctorID(ctx, dummyID, 20, 0); err != nil || len(appts) != 0 {
		t.Fatalf("expected empty appointments slice, got: %v, err: %v", appts, err)
	}
}

func TestClampPagination(t *testing.T) {
	tests := []struct {
		inLimit    int
		inOffset   int
		wantLimit  int
		wantOffset int
	}{
		{0, 0, 20, 0},
		{-10, -5, 20, 0},
		{50, 10, 50, 10},
		{150, 25, 100, 25},
		{100, 0, 100, 0},
	}

	for _, tt := range tests {
		gotLimit, gotOffset := clampPagination(tt.inLimit, tt.inOffset)
		if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
			t.Errorf("clampPagination(%d, %d) = (%d, %d), want (%d, %d)",
				tt.inLimit, tt.inOffset, gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
		}
	}
}
