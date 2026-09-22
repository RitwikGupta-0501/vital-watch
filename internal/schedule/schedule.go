package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// Standard time of day slots
const (
	SlotMorning   = "morning"
	SlotAfternoon = "afternoon"
	SlotEvening   = "evening"
	SlotBedtime   = "bedtime"
	SlotAsNeeded  = "as_needed"
)

var SlotOrder = map[string]int{
	SlotMorning:   1,
	SlotAfternoon: 2,
	SlotEvening:   3,
	SlotBedtime:   4,
	SlotAsNeeded:  5,
}

// ParseDailySlots converts clinical frequency and timing text into standardized daily dosage slots
func ParseDailySlots(frequency, timing string) []string {
	freq := strings.ToLower(strings.TrimSpace(frequency))
	timeStr := strings.ToLower(strings.TrimSpace(timing))
	combined := freq + " " + timeStr

	if strings.Contains(combined, "as needed") || strings.Contains(combined, "prn") || strings.Contains(combined, "when required") {
		return []string{SlotAsNeeded}
	}

	// 6 times daily / every 4 hours
	if strings.Contains(combined, "every 4 hours") || strings.Contains(combined, "every 4h") ||
		strings.Contains(combined, "q4h") || strings.Contains(combined, "6 times") {
		return []string{SlotMorning, SlotAfternoon, SlotEvening, SlotBedtime}
	}

	// 4 times daily
	if strings.Contains(combined, "four times") || strings.Contains(combined, "4 times") ||
		strings.Contains(combined, "qid") || strings.Contains(combined, "qds") ||
		strings.Contains(combined, "every 6 hours") || strings.Contains(combined, "every 6h") {
		return []string{SlotMorning, SlotAfternoon, SlotEvening, SlotBedtime}
	}

	// 3 times daily
	if strings.Contains(combined, "three times") || strings.Contains(combined, "3 times") ||
		strings.Contains(combined, "tid") || strings.Contains(combined, "tds") ||
		strings.Contains(combined, "every 8 hours") || strings.Contains(combined, "every 8h") {
		return []string{SlotMorning, SlotAfternoon, SlotEvening}
	}

	// 2 times daily
	if strings.Contains(combined, "twice") || strings.Contains(combined, "2 times") ||
		strings.Contains(combined, "bid") || strings.Contains(combined, "bd") ||
		strings.Contains(combined, "every 12 hours") || strings.Contains(combined, "every 12h") ||
		(strings.Contains(combined, "morning") && (strings.Contains(combined, "night") || strings.Contains(combined, "evening"))) {
		return []string{SlotMorning, SlotEvening}
	}

	// Specific single times
	if strings.Contains(combined, "bedtime") || strings.Contains(combined, "at night") || strings.Contains(combined, "before sleep") {
		return []string{SlotBedtime}
	}
	if strings.Contains(combined, "evening") || strings.Contains(combined, "dinner") {
		return []string{SlotEvening}
	}
	if strings.Contains(combined, "afternoon") || strings.Contains(combined, "lunch") {
		return []string{SlotAfternoon}
	}

	// Default for once daily / morning
	return []string{SlotMorning}
}

// BuildDailySchedule merges active prescription items with logged adherence records for a specific date
func BuildDailySchedule(
	patientID uuid.UUID,
	date time.Time,
	items []models.PrescriptionItem,
	existingLogs []models.MedicationLog,
) []models.MedicationLog {
	// Index existing logs by item_id:slot:dose_number
	logMap := make(map[string]models.MedicationLog)
	matchedLogIDs := make(map[uuid.UUID]bool)

	for _, l := range existingLogs {
		dNum := l.DoseNumber
		if dNum <= 0 {
			dNum = 1
		}
		key := fmt.Sprintf("%s:%s:%d", l.PrescriptionItemID.String(), l.TimeOfDay, dNum)
		logMap[key] = l
	}

	var results []models.MedicationLog
	for _, item := range items {
		slots := ParseDailySlots(item.Frequency, item.Timing)
		for _, slot := range slots {
			key := fmt.Sprintf("%s:%s:1", item.ID.String(), slot)
			if existing, found := logMap[key]; found {
				if existing.MedicationName == "" {
					existing.MedicationName = item.MedicationName
				}
				if existing.Dosage == "" {
					existing.Dosage = item.Dosage
				}
				if existing.Timing == "" {
					existing.Timing = item.Timing
				}
				if existing.MealTiming == "" {
					existing.MealTiming = item.Timing
				}
				if existing.Instructions == "" {
					existing.Instructions = item.Instructions
				}
				results = append(results, existing)
				matchedLogIDs[existing.ID] = true
			} else {
				// Virtual pending log entry
				results = append(results, models.MedicationLog{
					ID:                 uuid.Nil,
					PatientID:          patientID,
					PrescriptionItemID: item.ID,
					ScheduledDate:      date,
					TimeOfDay:          slot,
					DoseNumber:         1,
					MealTiming:         item.Timing,
					Status:             "pending",
					MedicationName:     item.MedicationName,
					Dosage:             item.Dosage,
					Timing:             item.Timing,
					Instructions:       item.Instructions,
				})
			}
		}
	}

	// Retain any existing logs that were not matched (e.g. multiple PRN doses, extra doses)
	for _, l := range existingLogs {
		if l.ID != uuid.Nil && !matchedLogIDs[l.ID] {
			results = append(results, l)
		}
	}

	// Sort chronologically by slot order, dose number, and medication name
	sort.SliceStable(results, func(i, j int) bool {
		orderI := SlotOrder[results[i].TimeOfDay]
		if orderI == 0 {
			orderI = 99
		}
		orderJ := SlotOrder[results[j].TimeOfDay]
		if orderJ == 0 {
			orderJ = 99
		}
		if orderI != orderJ {
			return orderI < orderJ
		}
		if results[i].DoseNumber != results[j].DoseNumber {
			return results[i].DoseNumber < results[j].DoseNumber
		}
		return results[i].MedicationName < results[j].MedicationName
	})

	return results
}
