package safety

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"
)

type AlertSeverity string

const (
	SeverityHigh     AlertSeverity = "HIGH"
	SeverityModerate AlertSeverity = "MODERATE"
	SeverityLow      AlertSeverity = "LOW"
)

type InteractionAlert struct {
	DrugA       string        `json:"drug_a"`
	DrugB       string        `json:"drug_b"`
	Severity    AlertSeverity `json:"severity"`
	Description string        `json:"description"`
	Source      string        `json:"source"`
}

type AllergyAlert struct {
	DrugName    string        `json:"drug_name"`
	Allergen    string        `json:"allergen"`
	Severity    AlertSeverity `json:"severity"`
	Description string        `json:"description"`
}

type SafetyReport struct {
	HasHighSeverityAlerts bool               `json:"has_high_severity_alerts"`
	ServiceDegraded       bool               `json:"service_degraded"`
	DegradedReason        string             `json:"degraded_reason,omitempty"`
	UncheckedDrugs        []string           `json:"unchecked_drugs,omitempty"`
	InteractionAlerts     []InteractionAlert `json:"interaction_alerts"`
	AllergyAlerts         []AllergyAlert     `json:"allergy_alerts"`
}

// Checker defines the interface for drug safety, interactions, and allergy verification
type Checker interface {
	CheckPrescriptionSafety(ctx context.Context, newMedications []string, activeMedications []string, patientAllergies []string) (*SafetyReport, error)
}

type cachedLabel struct {
	interactions      []string
	contraindications []string
	warnings          []string
	cachedAt          time.Time
}

// OpenFDAChecker implements clinical checks using OpenFDA drug label records with in-memory caching
type OpenFDAChecker struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	cache      map[string]cachedLabel
	cacheTTL   time.Duration
	mu         sync.RWMutex
	sf         singleflight.Group
}

const maxCacheEntries = 2000

// NewOpenFDAChecker creates a new safety checker instance
func NewOpenFDAChecker() *OpenFDAChecker {
	apiKey := strings.TrimSpace(os.Getenv("OPENFDA_API_KEY"))
	return &OpenFDAChecker{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    "https://api.fda.gov/drug/label.json",
		apiKey:     apiKey,
		cache:      make(map[string]cachedLabel),
		cacheTTL:   1 * time.Hour,
	}
}

// SetBaseURL allows overriding base URL for tests
func (c *OpenFDAChecker) SetBaseURL(u string) {
	c.baseURL = u
}

// SetHTTPClient allows overriding HTTP client for tests
func (c *OpenFDAChecker) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// SetAPIKey allows setting or overriding the OpenFDA API key
func (c *OpenFDAChecker) SetAPIKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = strings.TrimSpace(key)
}

func (c *OpenFDAChecker) putCache(key string, label cachedLabel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If entry already exists, simply update it without eviction
	if _, exists := c.cache[key]; !exists && len(c.cache) >= maxCacheEntries {
		now := time.Now()
		for k, v := range c.cache {
			if now.Sub(v.cachedAt) >= c.cacheTTL {
				delete(c.cache, k)
			}
		}
		// If still at capacity after expired eviction, batch-evict 5% of entries to avoid continuous thrashing
		if len(c.cache) >= maxCacheEntries {
			evictCount := maxCacheEntries / 20
			if evictCount < 1 {
				evictCount = 1
			}
			for k := range c.cache {
				delete(c.cache, k)
				evictCount--
				if evictCount <= 0 {
					break
				}
			}
		}
	}
	c.cache[key] = label
}

var dosageRegex = regexp.MustCompile(`(?i)\b\d+(\.\d+)?\s*(mg|mcg|g|ml|tablets?|capsules?|pills?|drops?|iu|meq|%)\b`)
var parenRegex = regexp.MustCompile(`\([^)]*\)`)
var formRegex = regexp.MustCompile(`(?i)\b(tablets?|capsules?|pills?|drops?|solution|syrup|suspension|injection|cream|ointment)\b`)
var saltRegex = regexp.MustCompile(`(?i)\b(hcl|hydrochloride|sodium|potassium|calcium|succinate|tartrate|maleate|fumarate|besylate|mesylate|phosphate|sulfate|nitrate|citrate|hydrate|monohydrate|dihydrate|trihydrate)\b`)

// CleanDrugName strips dosages, pharmaceutical salts, and extra annotations to isolate generic/brand chemical name
func CleanDrugName(name string) string {
	cleaned := parenRegex.ReplaceAllString(name, " ")
	cleaned = dosageRegex.ReplaceAllString(cleaned, " ")
	cleaned = formRegex.ReplaceAllString(cleaned, " ")
	cleaned = saltRegex.ReplaceAllString(cleaned, " ")
	cleaned = strings.ReplaceAll(cleaned, `"`, " ")
	cleaned = strings.ReplaceAll(cleaned, `\`, " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.ToLower(cleaned)
}

type sfResult struct {
	label cachedLabel
	found bool
}

func (c *OpenFDAChecker) getDrugLabel(ctx context.Context, drugName string) (cachedLabel, bool, error) {
	clean := CleanDrugName(drugName)
	if clean == "" {
		return cachedLabel{}, false, nil
	}

	c.mu.RLock()
	entry, exists := c.cache[clean]
	c.mu.RUnlock()

	if exists && time.Since(entry.cachedAt) < c.cacheTTL {
		if len(entry.interactions) == 0 && len(entry.contraindications) == 0 && len(entry.warnings) == 0 {
			// This was a 404 cache entry (empty)
			return cachedLabel{}, false, nil
		}
		return entry, true, nil
	}

	// Singleflight fetch
	res, err, _ := c.sf.Do(clean, func() (interface{}, error) {
		queryParam := fmt.Sprintf(`openfda.brand_name:"%s" OR openfda.generic_name:"%s"`, clean, clean)
		reqURL := fmt.Sprintf("%s?search=%s&limit=1", c.baseURL, url.QueryEscape(queryParam))
		if c.apiKey != "" {
			reqURL += fmt.Sprintf("&api_key=%s", url.QueryEscape(c.apiKey))
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			slog.WarnContext(ctx, "Failed to query OpenFDA", "drug", clean, "error", err)
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			// Drug not found in OpenFDA label database, cache empty to prevent repeated queries
			c.putCache(clean, cachedLabel{cachedAt: time.Now()})
			return sfResult{found: false}, nil
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			slog.WarnContext(ctx, "Non-200 from OpenFDA", "drug", clean, "status", resp.StatusCode, "response_body", string(body))
			return nil, fmt.Errorf("openfda service returned status %d", resp.StatusCode)
		}

		var fdaResp struct {
			Results []struct {
				DrugInteractions  []string `json:"drug_interactions"`
				Contraindications []string `json:"contraindications"`
				Warnings          []string `json:"warnings"`
			} `json:"results"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&fdaResp); err != nil {
			return nil, err
		}

		label := cachedLabel{cachedAt: time.Now()}
		if len(fdaResp.Results) > 0 {
			label.interactions = fdaResp.Results[0].DrugInteractions
			label.contraindications = fdaResp.Results[0].Contraindications
			label.warnings = fdaResp.Results[0].Warnings
		}

		c.putCache(clean, label)

		return sfResult{label: label, found: true}, nil
	})

	if err != nil {
		return cachedLabel{}, false, err
	}

	sfr := res.(sfResult)
	return sfr.label, sfr.found, nil
}

func matchAllergen(cleanMed string, cleanAllergen string, re *regexp.Regexp) bool {
	if cleanMed == "" || cleanAllergen == "" {
		return false
	}
	if re != nil {
		return re.MatchString(cleanMed)
	}
	return strings.EqualFold(cleanMed, cleanAllergen)
}

func canonicalPairKey(a, b string) string {
	if a < b {
		return a + "<->" + b
	}
	return b + "<->" + a
}

// CheckPrescriptionSafety analyzes new medications against active ones and allergies
func (c *OpenFDAChecker) CheckPrescriptionSafety(ctx context.Context, newMedications []string, activeMedications []string, patientAllergies []string) (*SafetyReport, error) {
	report := &SafetyReport{
		InteractionAlerts: make([]InteractionAlert, 0),
		AllergyAlerts:     make([]AllergyAlert, 0),
	}

	// 1. Compile regexes for allergies (N)
	allergyRegexes := make(map[string]*regexp.Regexp, len(patientAllergies))
	for _, allergen := range patientAllergies {
		cleanAllergen := strings.ToLower(strings.TrimSpace(allergen))
		if cleanAllergen == "" {
			continue
		}
		pattern := `(?i)\b` + regexp.QuoteMeta(cleanAllergen) + `\b`
		if re, err := regexp.Compile(pattern); err == nil {
			allergyRegexes[allergen] = re
		}
	}

	// 2. Check Allergy Contraindications with word boundaries
	for _, newMed := range newMedications {
		cleanNew := CleanDrugName(newMed)
		if cleanNew == "" {
			continue
		}
		for _, allergen := range patientAllergies {
			re := allergyRegexes[allergen]
			cleanAllergen := strings.ToLower(strings.TrimSpace(allergen))
			if matchAllergen(cleanNew, cleanAllergen, re) {
				report.AllergyAlerts = append(report.AllergyAlerts, AllergyAlert{
					DrugName:    newMed,
					Allergen:    allergen,
					Severity:    SeverityHigh,
					Description: fmt.Sprintf("Patient has a documented allergy to %q, matching prescribed medication %q.", allergen, newMed),
				})
				report.HasHighSeverityAlerts = true
			}
		}
	}

	// 3. Concurrently pre-fetch OpenFDA labels for all unique medications to avoid sequential network latency
	uniqueDrugs := make(map[string]struct{})
	for _, m := range newMedications {
		if cName := CleanDrugName(m); cName != "" {
			uniqueDrugs[cName] = struct{}{}
		}
	}
	for _, m := range activeMedications {
		if cName := CleanDrugName(m); cName != "" {
			uniqueDrugs[cName] = struct{}{}
		}
	}

	degradedDrugs := make(map[string]struct{})
	var degradedMu sync.Mutex

	recordDegraded := func(drugName string) {
		degradedMu.Lock()
		degradedDrugs[drugName] = struct{}{}
		degradedMu.Unlock()
	}

	if len(uniqueDrugs) > 0 {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)
		for drugName := range uniqueDrugs {
			wg.Add(1)
			go func(d string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				_, _, err := c.getDrugLabel(ctx, d)
				if err != nil {
					recordDegraded(d)
				}
			}(drugName)
		}
		wg.Wait()
	}

	// 4. Check Drug-Drug Interactions (between new medications, and new vs active)
	allDrugsToCompareAgainst := make([]string, 0, len(newMedications)+len(activeMedications))
	allDrugsToCompareAgainst = append(allDrugsToCompareAgainst, activeMedications...)
	allDrugsToCompareAgainst = append(allDrugsToCompareAgainst, newMedications...)

	// Pre-compile interaction regexes
	targetRegexes := make(map[string]*regexp.Regexp)
	for _, targetDrug := range allDrugsToCompareAgainst {
		cleanTarget := strings.TrimSpace(CleanDrugName(targetDrug))
		if cleanTarget != "" && targetRegexes[cleanTarget] == nil {
			pattern := `(?i)\b` + regexp.QuoteMeta(cleanTarget) + `\b`
			if re, err := regexp.Compile(pattern); err == nil {
				targetRegexes[cleanTarget] = re
			}
		}
	}

	checkedPairs := make(map[string]bool)

	for _, primaryDrug := range newMedications {
		cleanPrimary := CleanDrugName(primaryDrug)
		if cleanPrimary == "" {
			continue
		}

		for _, secondaryDrug := range allDrugsToCompareAgainst {
			cleanSecondary := CleanDrugName(secondaryDrug)
			if cleanSecondary == "" || cleanPrimary == cleanSecondary {
				continue
			}

			pairKey := canonicalPairKey(cleanPrimary, cleanSecondary)
			if checkedPairs[pairKey] {
				continue
			}
			checkedPairs[pairKey] = true

			// Check label of primary drug first
			labelPrimary, foundPrimary, errPrimary := c.getDrugLabel(ctx, cleanPrimary)
			if errPrimary != nil {
				recordDegraded(cleanPrimary)
			}
			
			secondaryRe := targetRegexes[cleanSecondary]

			var interactionFoundPrimary, isHighPrimary bool
			if foundPrimary && secondaryRe != nil {
				interactionFoundPrimary, isHighPrimary = c.scanLabelForInteraction(report, labelPrimary, primaryDrug, secondaryDrug, secondaryRe)
			}

			// HIGH-1: If primary drug did not yield a High severity contraindication, check reciprocal (secondary drug label)
			if !isHighPrimary {
				labelSecondary, foundSecondary, errSecondary := c.getDrugLabel(ctx, cleanSecondary)
				if errSecondary != nil {
					recordDegraded(cleanSecondary)
				}
				primaryRe := targetRegexes[cleanPrimary]
				if foundSecondary && primaryRe != nil {
					c.scanLabelForReciprocalInteraction(report, labelSecondary, secondaryDrug, primaryDrug, primaryRe, interactionFoundPrimary)
				}
			}
		}
	}

	if len(degradedDrugs) > 0 {
		report.ServiceDegraded = true
		report.HasHighSeverityAlerts = true
		report.DegradedReason = "OpenFDA drug interaction service is unreachable or rate limited; automated interaction screening is incomplete."
		report.UncheckedDrugs = make([]string, 0, len(degradedDrugs))
		for d := range degradedDrugs {
			report.UncheckedDrugs = append(report.UncheckedDrugs, d)
		}
		report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
			DrugA:       strings.Join(report.UncheckedDrugs, ", "),
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("CRITICAL SAFETY WARNING: Automated interaction checks could not be completed for [%s] due to external clinical API unavailability. Clinician verification and explicit override are required prior to dispensing.", strings.Join(report.UncheckedDrugs, ", ")),
			Source:      "Clinical Safety Guardrail (Service Degraded)",
		})
	}

	return report, nil
}

func (c *OpenFDAChecker) scanLabelForInteraction(report *SafetyReport, label cachedLabel, drugA, drugB string, targetRe *regexp.Regexp) (found bool, isHigh bool) {
	if targetRe == nil {
		return false, false
	}

	// 1. Check Contraindications (High severity)
	for _, contra := range label.contraindications {
		if matchSnippet := findInteractionSnippetWithRegex(contra, targetRe); matchSnippet != "" {
			report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
				DrugA:       drugA,
				DrugB:       drugB,
				Severity:    SeverityHigh,
				Description: matchSnippet,
				Source:      "OpenFDA Label: Contraindications",
			})
			report.HasHighSeverityAlerts = true
			return true, true
		}
	}

	// 2. Check Drug Interactions (Moderate severity)
	for _, inter := range label.interactions {
		if matchSnippet := findInteractionSnippetWithRegex(inter, targetRe); matchSnippet != "" {
			report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
				DrugA:       drugA,
				DrugB:       drugB,
				Severity:    SeverityModerate,
				Description: matchSnippet,
				Source:      "OpenFDA Label: Drug Interactions",
			})
			return true, false
		}
	}

	return false, false
}

func (c *OpenFDAChecker) scanLabelForReciprocalInteraction(report *SafetyReport, label cachedLabel, drugA, drugB string, targetRe *regexp.Regexp, alreadyFoundModerate bool) (found bool, isHigh bool) {
	if targetRe == nil {
		return false, false
	}

	// 1. Always check Contraindications (High severity) across the reciprocal label
	for _, contra := range label.contraindications {
		if matchSnippet := findInteractionSnippetWithRegex(contra, targetRe); matchSnippet != "" {
			report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
				DrugA:       drugA,
				DrugB:       drugB,
				Severity:    SeverityHigh,
				Description: matchSnippet,
				Source:      "OpenFDA Label: Contraindications",
			})
			report.HasHighSeverityAlerts = true
			return true, true
		}
	}

	// 2. Check Drug Interactions (Moderate severity) only if not already discovered on the primary label
	if !alreadyFoundModerate {
		for _, inter := range label.interactions {
			if matchSnippet := findInteractionSnippetWithRegex(inter, targetRe); matchSnippet != "" {
				report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
					DrugA:       drugA,
					DrugB:       drugB,
					Severity:    SeverityModerate,
					Description: matchSnippet,
					Source:      "OpenFDA Label: Drug Interactions",
				})
				return true, false
			}
		}
	}

	return false, false
}

// findInteractionSnippet searches text for mention of target drug using word boundaries and extracts a relevant sentence
func findInteractionSnippet(text, targetDrug string) string {
	cleanTarget := strings.TrimSpace(targetDrug)
	if cleanTarget == "" {
		return ""
	}

	pattern := `(?i)\b` + regexp.QuoteMeta(cleanTarget) + `\b`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}

	return findInteractionSnippetWithRegex(text, re)
}

// findInteractionSnippetWithRegex uses a precompiled regex and extracts a surrounding snippet along rune boundaries
func findInteractionSnippetWithRegex(text string, re *regexp.Regexp) string {
	if re == nil || text == "" {
		return ""
	}

	loc := re.FindStringIndex(text)
	if loc == nil {
		return ""
	}

	runes := []rune(text)
	byteIdx := loc[0]
	byteEnd := loc[1]

	// Map byte indices to rune indices to ensure UTF-8 validity
	runeStart := 0
	runeEnd := 0
	currentByte := 0

	for i, r := range runes {
		if currentByte == byteIdx {
			runeStart = i
		}
		currentByte += utf8.RuneLen(r)
		if currentByte >= byteEnd && runeEnd == 0 {
			runeEnd = i + 1
		}
	}
	if runeEnd == 0 {
		runeEnd = len(runes)
	}

	// Extract surrounding snippet up to ~80 runes before and ~120 runes after
	sStart := runeStart - 80
	if sStart < 0 {
		sStart = 0
	}
	sEnd := runeEnd + 120
	if sEnd > len(runes) {
		sEnd = len(runes)
	}

	snippet := strings.TrimSpace(string(runes[sStart:sEnd]))
	if sStart > 0 {
		snippet = "..." + snippet
	}
	if sEnd < len(runes) {
		snippet = snippet + "..."
	}

	return snippet
}
