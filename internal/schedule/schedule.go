package schedule

import (
	"fmt"
	"regexp"
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

var (
	rePRN            = regexp.MustCompile(`(?i)\b(as needed|prn|when required|as directed)\b`)
	reQ4H            = regexp.MustCompile(`(?i)\b(every 4 hours|every 4\s*h|q4h|6 times daily|6 times a day|6x daily)\b`)
	reQID            = regexp.MustCompile(`(?i)\b(four times daily|four times a day|4 times daily|4 times a day|4x daily|qid|qds|every 6 hours|every 6\s*h|q6h)\b`)
	reTID            = regexp.MustCompile(`(?i)\b(three times daily|three times a day|3 times daily|3 times a day|3x daily|tid|tds|every 8 hours|every 8\s*h|q8h)\b`)
	reBID            = regexp.MustCompile(`(?i)\b(twice daily|twice a day|2 times daily|2 times a day|2x daily|bid|bd|every 12 hours|every 12\s*h|q12h)\b`)
	reMorningEvening = regexp.MustCompile(`(?i)(\bmorning\b.*\b(night|evening)\b|\b(night|evening)\b.*\bmorning\b)`)
	reBedtime        = regexp.MustCompile(`(?i)\b(bedtime|at night|before sleep|hs|qhs)\b`)
	reEvening        = regexp.MustCompile(`(?i)\b(evening|dinner|supper)\b`)
	reAfternoon      = regexp.MustCompile(`(?i)\b(afternoon|lunch|midday)\b`)
	reMorning        = regexp.MustCompile(`(?i)\b(morning|breakfast|qam)\b`)
)

// ParseDailySlots converts clinical frequency and timing text into standardized daily dosage slots
// using word-boundary regular expressions to eliminate false-positive substring matches on clinical terms.
func ParseDailySlots(frequency, timing string) []string {
	freq := strings.TrimSpace(frequency)
	timeStr := strings.TrimSpace(timing)
	combined := freq + " " + timeStr

	if rePRN.MatchString(combined) {
		return []string{SlotAsNeeded}
	}

	// 6 times daily / every 4 hours
	if reQ4H.MatchString(combined) {
		return []string{SlotMorning, SlotAfternoon, SlotEvening, SlotBedtime}
	}

	// 4 times daily
	if reQID.MatchString(combined) {
		return []string{SlotMorning, SlotAfternoon, SlotEvening, SlotBedtime}
	}

	// 3 times daily
	if reTID.MatchString(combined) {
		return []string{SlotMorning, SlotAfternoon, SlotEvening}
	}

	// 2 times daily
	if reBID.MatchString(combined) || reMorningEvening.MatchString(combined) {
		return []string{SlotMorning, SlotEvening}
	}

	// Specific single times
	if reBedtime.MatchString(combined) {
		return []string{SlotBedtime}
	}
	if reEvening.MatchString(combined) {
		return []string{SlotEvening}
	}
	if reAfternoon.MatchString(combined) {
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

	itemMap := make(map[uuid.UUID]models.PrescriptionItem)
	for _, item := range items {
		itemMap[item.ID] = item
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
					TimeOfDay:          models.MedicationTimeOfDay(slot),
					DoseNumber:         1,
					MealTiming:         item.Timing,
					Status:             models.MedicationLogStatusPending,
					MedicationName:     item.MedicationName,
					Dosage:             item.Dosage,
					Timing:             item.Timing,
					Instructions:       item.Instructions,
				})
			}
		}
	}

	// Retain any existing logs that were not matched (e.g. multiple PRN doses, extra doses)
	// and enrich with prescription item metadata if missing
	for _, l := range existingLogs {
		if l.ID != uuid.Nil && !matchedLogIDs[l.ID] {
			if item, exists := itemMap[l.PrescriptionItemID]; exists {
				if l.MedicationName == "" {
					l.MedicationName = item.MedicationName
				}
				if l.Dosage == "" {
					l.Dosage = item.Dosage
				}
				if l.Timing == "" {
					l.Timing = item.Timing
				}
				if l.MealTiming == "" {
					l.MealTiming = item.Timing
				}
				if l.Instructions == "" {
					l.Instructions = item.Instructions
				}
			}
			results = append(results, l)
		}
	}

	// Sort chronologically by slot order, dose number, and medication name
	sort.SliceStable(results, func(i, j int) bool {
		orderI := SlotOrder[string(results[i].TimeOfDay)]
		if orderI == 0 {
			orderI = 99
		}
		orderJ := SlotOrder[string(results[j].TimeOfDay)]
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
