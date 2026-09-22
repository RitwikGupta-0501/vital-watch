package schedule

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

func TestParseDailySlots(t *testing.T) {
	tests := []struct {
		name      string
		frequency string
		timing    string
		expected  []string
	}{
		{
			name:      "Twice daily",
			frequency: "Twice daily",
			timing:    "After food",
			expected:  []string{"morning", "evening"},
		},
		{
			name:      "BID abbreviation",
			frequency: "1 tab BID",
			timing:    "",
			expected:  []string{"morning", "evening"},
		},
		{
			name:      "Three times daily",
			frequency: "Three times a day",
			timing:    "Before meals",
			expected:  []string{"morning", "afternoon", "evening"},
		},
		{
			name:      "QDS / 4 times daily",
			frequency: "QDS",
			timing:    "",
			expected:  []string{"morning", "afternoon", "evening", "bedtime"},
		},
		{
			name:      "Bedtime medication",
			frequency: "Once daily",
			timing:    "At bedtime",
			expected:  []string{"bedtime"},
		},
		{
			name:      "PRN / As needed",
			frequency: "As needed for pain",
			timing:    "PRN",
			expected:  []string{"as_needed"},
		},
		{
			name:      "Default once daily",
			frequency: "Once daily",
			timing:    "Morning",
			expected:  []string{"morning"},
		},
		{
			name:      "Q4H 6 times daily",
			frequency: "Every 4 hours",
			timing:    "with water",
			expected:  []string{"morning", "afternoon", "evening", "bedtime"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDailySlots(tt.frequency, tt.timing)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected len %d, got len %d (%v)", len(tt.expected), len(got), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("slot [%d] expected %s, got %s", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestBuildDailySchedule(t *testing.T) {
	patientID := uuid.New()
	item1ID := uuid.New()
	item2ID := uuid.New()
	now := time.Now()

	items := []models.PrescriptionItem{
		{
			ID:             item1ID,
			MedicationName: "Metformin",
			Dosage:         "500mg",
			Frequency:      "twice daily",
			Timing:         "with meals",
		},
		{
			ID:             item2ID,
			MedicationName: "Atorvastatin",
			Dosage:         "20mg",
			Frequency:      "once daily",
			Timing:         "at bedtime",
		},
	}

	// Metformin morning dose was already logged as taken
	takenTime := now.Add(-2 * time.Hour)
	existingLogs := []models.MedicationLog{
		{
			ID:                 uuid.New(),
			PatientID:          patientID,
			PrescriptionItemID: item1ID,
			ScheduledDate:      now,
			TimeOfDay:          "morning",
			DoseNumber:         1,
			Status:             "taken",
			TakenAt:            &takenTime,
		},
	}

	sched := BuildDailySchedule(patientID, now, items, existingLogs)

	// Expected slots: Metformin morning (taken), Metformin evening (pending), Atorvastatin bedtime (pending)
	if len(sched) != 3 {
		t.Fatalf("expected 3 schedule items, got %d", len(sched))
	}

	// 1. Verify chronological sorting: morning -> evening -> bedtime
	if sched[0].TimeOfDay != "morning" {
		t.Errorf("expected slot 0 to be morning, got %s", sched[0].TimeOfDay)
	}
	if sched[1].TimeOfDay != "evening" {
		t.Errorf("expected slot 1 to be evening, got %s", sched[1].TimeOfDay)
	}
	if sched[2].TimeOfDay != "bedtime" {
		t.Errorf("expected slot 2 to be bedtime, got %s", sched[2].TimeOfDay)
	}

	// 2. Verify timing / meal timing metadata is preserved
	if sched[0].Timing != "with meals" || sched[0].MealTiming != "with meals" {
		t.Errorf("expected timing with meals, got timing=%s, meal_timing=%s", sched[0].Timing, sched[0].MealTiming)
	}
	if sched[2].Timing != "at bedtime" {
		t.Errorf("expected timing at bedtime, got %s", sched[2].Timing)
	}

	var foundTaken bool
	for _, entry := range sched {
		if entry.PrescriptionItemID == item1ID && entry.TimeOfDay == "morning" {
			if entry.Status != "taken" {
				t.Errorf("expected taken status for morning metformin, got %s", entry.Status)
			}
			if entry.MedicationName != "Metformin" {
				t.Errorf("expected medication name Metformin, got %s", entry.MedicationName)
			}
			foundTaken = true
		}
	}
	if !foundTaken {
		t.Error("did not find taken morning metformin log entry")
	}
}

func TestBuildDailySchedule_MultiplePRNDosesRetained(t *testing.T) {
	patientID := uuid.New()
	prnItemID := uuid.New()
	now := time.Now()

	items := []models.PrescriptionItem{
		{
			ID:             prnItemID,
			MedicationName: "Albuterol Inhaler",
			Dosage:         "2 puffs",
			Frequency:      "as needed",
			Timing:         "before exercise",
		},
	}

	// Patient took 2 PRN doses today
	t1 := now.Add(-4 * time.Hour)
	t2 := now.Add(-1 * time.Hour)
	existingLogs := []models.MedicationLog{
		{
			ID:                 uuid.New(),
			PatientID:          patientID,
			PrescriptionItemID: prnItemID,
			ScheduledDate:      now,
			TimeOfDay:          "as_needed",
			DoseNumber:         1,
			Status:             "taken",
			TakenAt:            &t1,
			MedicationName:     "Albuterol Inhaler",
		},
		{
			ID:                 uuid.New(),
			PatientID:          patientID,
			PrescriptionItemID: prnItemID,
			ScheduledDate:      now,
			TimeOfDay:          "as_needed",
			DoseNumber:         2,
			Status:             "taken",
			TakenAt:            &t2,
			MedicationName:     "Albuterol Inhaler",
		},
	}

	sched := BuildDailySchedule(patientID, now, items, existingLogs)

	// Both PRN doses must be retained in the schedule
	if len(sched) != 2 {
		t.Fatalf("expected 2 PRN schedule entries, got %d", len(sched))
	}
	if sched[0].DoseNumber != 1 || sched[1].DoseNumber != 2 {
		t.Errorf("expected dose numbers 1 and 2, got %d and %d", sched[0].DoseNumber, sched[1].DoseNumber)
	}
	if sched[0].Timing != "before exercise" {
		t.Errorf("expected timing before exercise, got %s", sched[0].Timing)
	}
}
