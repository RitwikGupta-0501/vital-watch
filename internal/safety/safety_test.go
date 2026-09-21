package safety

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCleanDrugName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Amoxicillin 500mg", "amoxicillin"},
		{"Metformin 1000 mg", "metformin"},
		{"Aspirin (oral)", "aspirin"},
		{"Atorvastatin 20mg tablet", "atorvastatin"},
		{"Vitamin D3 1000iu", "vitamin d3"},
	}

	for _, tt := range tests {
		got := CleanDrugName(tt.input)
		if got != tt.expected {
			t.Errorf("CleanDrugName(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestAllergyContraindication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Amoxicillin 500mg", "Paracetamol 650mg"},
		[]string{},
		[]string{"Amoxicillin"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.HasHighSeverityAlerts {
		t.Errorf("expected high severity alert for allergy")
	}

	if len(report.AllergyAlerts) != 1 {
		t.Fatalf("expected 1 allergy alert, got %d", len(report.AllergyAlerts))
	}
	if report.AllergyAlerts[0].Allergen != "Amoxicillin" {
		t.Errorf("expected allergy to Amoxicillin, got %s", report.AllergyAlerts[0].Allergen)
	}
}

func TestDrugDrugInteraction_MockOpenFDA(t *testing.T) {
	// Mock OpenFDA server returning interaction label for Warfarin
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"drug_interactions": []string{
						"Concomitant use of aspirin with warfarin increases the risk of major bleeding.",
					},
					"contraindications": []string{
						"Coadministration with high-dose aspirin is strictly contraindicated.",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Warfarin 5mg"},
		[]string{"Aspirin 81mg"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.InteractionAlerts) == 0 {
		t.Fatalf("expected interaction alerts between Warfarin and Aspirin, got none")
	}

	foundHigh := false
	for _, alert := range report.InteractionAlerts {
		if alert.Severity == SeverityHigh {
			foundHigh = true
		}
	}

	if !foundHigh {
		t.Errorf("expected at least one HIGH severity interaction alert")
	}
}

func TestFailOpenOnFDAError(t *testing.T) {
	// Mock OpenFDA server failing with 503
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "FDA API service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	// Should not fail or crash, but return a report without blocking
	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Lisinopril 10mg"},
		[]string{"Potassium 20mEq"},
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error on FDA 503 (fail-open), got %v", err)
	}
	if report == nil {
		t.Fatalf("expected non-nil report")
	}
}

func TestFindInteractionSnippet_WordBoundaries(t *testing.T) {
	// Substring false-positive test cases:
	// "iron" should NOT match "environmental factors and cirrhosis"
	text := "Patients with environmental allergies and cirrhosis had no adverse effects."
	if snippet := findInteractionSnippet(text, "iron"); snippet != "" {
		t.Fatalf("expected no snippet for 'iron' in 'environmental/cirrhosis', got: %s", snippet)
	}

	// "ace" should NOT match "placebo" or "acetaminophen"
	text2 := "In placebo controlled trials, no adverse reactions were observed."
	if snippet := findInteractionSnippet(text2, "ace"); snippet != "" {
		t.Fatalf("expected no snippet for 'ace' in 'placebo', got: %s", snippet)
	}

	// "tin" should NOT match "continue"
	text3 := "Discontinue medication if severe rash occurs."
	if snippet := findInteractionSnippet(text3, "tin"); snippet != "" {
		t.Fatalf("expected no snippet for 'tin' in 'discontinue', got: %s", snippet)
	}

	// Legitimate standalone match should match
	text4 := "Concurrent administration with aspirin increases bleeding risk."
	snippet := findInteractionSnippet(text4, "aspirin")
	if snippet == "" {
		t.Fatalf("expected snippet match for 'aspirin'")
	}
}

func TestReciprocalDrugDrugInteraction(t *testing.T) {
	// Drug A (DrugOne) returns no interactions in its label
	// Drug B (DrugTwo) returns a contraindication explicitly mentioning DrugOne
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("search")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "drugtwo") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"contraindications": []string{
							"Concomitant use of drugtwo with drugone is strictly contraindicated due to toxicity.",
						},
					},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"contraindications": []string{},
						"drug_interactions":  []string{},
					},
				},
			})
		}
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"DrugOne 10mg", "DrugTwo 20mg"},
		[]string{},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.HasHighSeverityAlerts {
		t.Fatalf("expected high severity alert via reciprocal label lookup")
	}
	if len(report.InteractionAlerts) != 1 {
		t.Fatalf("expected exactly 1 interaction alert, got %d", len(report.InteractionAlerts))
	}
}

func TestAllergyWordBoundaries(t *testing.T) {
	checker := NewOpenFDAChecker()

	// Allergen "ace" should not flag "acetaminophen"
	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Acetaminophen 500mg"},
		nil,
		[]string{"ace"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.AllergyAlerts) != 0 {
		t.Fatalf("expected 0 allergy alerts for ace matching acetaminophen, got %d", len(report.AllergyAlerts))
	}

	// Allergen "penicillin" should flag "Penicillin V"
	report2, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Penicillin V 250mg"},
		nil,
		[]string{"penicillin"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report2.AllergyAlerts) != 1 {
		t.Fatalf("expected 1 allergy alert for penicillin matching Penicillin V, got %d", len(report2.AllergyAlerts))
	}
}

func TestOpenFDAQueryEncoding_NoDoubleEscapePlus(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	_, _ = checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Warfarin"},
		nil,
		nil,
	)

	if strings.Contains(receivedQuery, "%2BOR%2B") {
		t.Fatalf("query string contains double-escaped plus: %s", receivedQuery)
	}
}

func TestOpenFDA_APIKeyAppended(t *testing.T) {
	var receivedRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)
	checker.SetAPIKey("test-fda-key-12345")

	_, _ = checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"Aspirin", "Warfarin"},
		nil,
		nil,
	)

	if !strings.Contains(receivedRawQuery, "api_key=test-fda-key-12345") {
		t.Fatalf("expected api_key in query string, got: %s", receivedRawQuery)
	}
}

func TestFindInteractionSnippet_UTF8Runes(t *testing.T) {
	// Label containing multi-byte UTF-8 characters (German, French, symbols)
	text := "Gleichzeitige Verabreichung von Aspirin führt zu erhöhter Blutungsgefahr — besondere Vorsicht bei älteren Patienten."
	snippet := findInteractionSnippet(text, "Aspirin")

	if snippet == "" {
		t.Fatalf("expected match for Aspirin")
	}

	// Verify the snippet is 100% valid UTF-8 and hasn't severed multi-byte runes
	if !utf8.ValidString(snippet) {
		t.Fatalf("extracted snippet contains invalid UTF-8 bytes: %q", snippet)
	}
}

func TestCacheEvictionAtCapacity(t *testing.T) {
	checker := NewOpenFDAChecker()

	// Fill cache past max capacity with dummy entries
	for i := 0; i < maxCacheEntries+10; i++ {
		checker.putCache(fmt.Sprintf("drug-%d", i), cachedLabel{
			cachedAt: time.Now().Add(-2 * time.Hour), // Expired
		})
	}

	checker.mu.RLock()
	cacheLen := len(checker.cache)
	checker.mu.RUnlock()

	if cacheLen > maxCacheEntries {
		t.Fatalf("cache length %d exceeds maxCacheEntries %d", cacheLen, maxCacheEntries)
	}
}

func TestNonInteractingPairs_CheckedOnce(t *testing.T) {
	var requestedSearches []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		requestedSearches = append(requestedSearches, q)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"contraindications": []string{},
					"drug_interactions": []string{},
				},
			},
		})
	}))
	defer server.Close()

	checker := NewOpenFDAChecker()
	checker.SetBaseURL(server.URL)

	report, err := checker.CheckPrescriptionSafety(
		context.Background(),
		[]string{"DrugA 10mg", "DrugB 20mg"},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.InteractionAlerts) != 0 {
		t.Fatalf("expected 0 interaction alerts, got %d", len(report.InteractionAlerts))
	}

	if len(requestedSearches) != 2 {
		t.Fatalf("expected exactly 2 FDA queries for 2 distinct drugs, got %d: %v", len(requestedSearches), requestedSearches)
	}
}
