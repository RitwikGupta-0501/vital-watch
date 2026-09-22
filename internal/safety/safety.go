package safety

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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

// CleanDrugName strips dosages and extra annotations to isolate generic/brand chemical name
func CleanDrugName(name string) string {
	cleaned := parenRegex.ReplaceAllString(name, " ")
	cleaned = dosageRegex.ReplaceAllString(cleaned, " ")
	cleaned = formRegex.ReplaceAllString(cleaned, " ")
	cleaned = strings.ReplaceAll(cleaned, `"`, " ")
	cleaned = strings.ReplaceAll(cleaned, `\`, " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.ToLower(cleaned)
}

func (c *OpenFDAChecker) getDrugLabel(ctx context.Context, drugName string) (*cachedLabel, error) {
	clean := CleanDrugName(drugName)
	if clean == "" {
		return nil, nil
	}

	c.mu.RLock()
	entry, exists := c.cache[clean]
	c.mu.RUnlock()

	if exists && time.Since(entry.cachedAt) < c.cacheTTL {
		return &entry, nil
	}

	// Query OpenFDA: search by brand_name or generic_name
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
		log.Printf("[OpenFDA DDI] Warning: Failed to query OpenFDA for %q: %v (failing open)", clean, err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Drug not found in OpenFDA label database, cache empty to prevent repeated queries
		c.putCache(clean, cachedLabel{cachedAt: time.Now()})
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[OpenFDA DDI] Non-200 from OpenFDA for %q (status %d): %s", clean, resp.StatusCode, string(body))
		return nil, nil
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

	return &label, nil
}

func matchAllergen(cleanMed, allergen string) bool {
	cleanAllergen := strings.ToLower(strings.TrimSpace(allergen))
	if cleanMed == "" || cleanAllergen == "" {
		return false
	}
	pattern := `(?i)\b` + regexp.QuoteMeta(cleanAllergen) + `\b`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return strings.EqualFold(cleanMed, cleanAllergen)
	}
	return re.MatchString(cleanMed)
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

	// 1. Check Allergy Contraindications with word boundaries
	for _, newMed := range newMedications {
		cleanNew := CleanDrugName(newMed)
		if cleanNew == "" {
			continue
		}
		for _, allergen := range patientAllergies {
			if matchAllergen(cleanNew, allergen) {
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

	// 2. Check Drug-Drug Interactions (between new medications, and new vs active)
	allDrugsToCompareAgainst := make([]string, 0, len(newMedications)+len(activeMedications))
	allDrugsToCompareAgainst = append(allDrugsToCompareAgainst, activeMedications...)
	allDrugsToCompareAgainst = append(allDrugsToCompareAgainst, newMedications...)

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
			labelPrimary, _ := c.getDrugLabel(ctx, cleanPrimary)
			alertFound := c.scanLabelForInteraction(report, labelPrimary, primaryDrug, secondaryDrug, cleanSecondary)

			// If no interaction found in primary drug label, check reciprocal (secondary drug label)
			if !alertFound {
				labelSecondary, _ := c.getDrugLabel(ctx, cleanSecondary)
				c.scanLabelForInteraction(report, labelSecondary, secondaryDrug, primaryDrug, cleanPrimary)
			}
		}
	}

	return report, nil
}

func (c *OpenFDAChecker) scanLabelForInteraction(report *SafetyReport, label *cachedLabel, drugA, drugB, cleanTarget string) bool {
	if label == nil {
		return false
	}
	target := strings.TrimSpace(cleanTarget)
	if target == "" {
		return false
	}

	pattern := `(?i)\b` + regexp.QuoteMeta(target) + `\b`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}

	// 1. Check Contraindications (High severity)
	for _, contra := range label.contraindications {
		if matchSnippet := findInteractionSnippetWithRegex(contra, re); matchSnippet != "" {
			report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
				DrugA:       drugA,
				DrugB:       drugB,
				Severity:    SeverityHigh,
				Description: matchSnippet,
				Source:      "OpenFDA Label: Contraindications",
			})
			report.HasHighSeverityAlerts = true
			return true
		}
	}

	// 2. Check Drug Interactions (Moderate severity)
	for _, inter := range label.interactions {
		if matchSnippet := findInteractionSnippetWithRegex(inter, re); matchSnippet != "" {
			report.InteractionAlerts = append(report.InteractionAlerts, InteractionAlert{
				DrugA:       drugA,
				DrugB:       drugB,
				Severity:    SeverityModerate,
				Description: matchSnippet,
				Source:      "OpenFDA Label: Drug Interactions",
			})
			return true
		}
	}

	return false
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
