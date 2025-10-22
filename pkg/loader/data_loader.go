package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type WorkURN struct {
	Simple string // for case where a work corresponds to one alphanumeric URN
	// case where work corresponds to a range of URNs, e.g. Dem. or. for the range of orations of Demosthanes:
	Range *WorkRange // only one of Simple and Range is relevant in any given case
}

type WorkRange struct {
	Prefix string // frequently tlg or phi for Greek and Latin respectively
	Start  int
	End    int
}

// Data structures matching the Python dictionaries in atlas_data_prep code
type GreekData struct {
	AuthAbb           map[string]string             `json:"GREEK_AUTH_ABB"`
	WorkURNs          map[string]map[string]WorkURN `json:"GREEK_WORK_URNS"`
	AuthURNs          map[string]string             `json:"GREEK_AUTH_URNS"`
	SingleWorkAuthors []string                      `json:"GREEK_SINGLE_WORK_AUTHORS"`
}

// note that AuthAbb maps to an interface rather than a string in order to map
// to a function to handle authors like Pliny and Seneca Elder vs. Younger.
type LatinData struct {
	AuthAbb           map[string]any                `json:"LATIN_AUTH_ABB"`
	WorkURNs          map[string]map[string]WorkURN `json:"LATIN_WORK_URNS"`
	AuthURNs          map[string]string             `json:"LATIN_AUTH_URNS"`
	SingleWorkAuthors []string                      `json:"LATIN_SINGLE_WORK_AUTHORS"`
}

type ScholData struct {
	AuthAbb  map[string]string             `json:"SCHOL_AUTH_ABB"`
	WorkURNs map[string]map[string]WorkURN `json:"SCHOL_WORK_URNS"`
	AuthURNs map[string]string             `json:"SCHOL_AUTH_URNS"`
}

type OtherData struct {
	AuthAbb  map[string]string             `json:"OTHER_AUTH_ABB"`
	WorkURNs map[string]map[string]WorkURN `json:"OTHER_WORK_URNS"`
	AuthURNs map[string]string             `json:"OTHER_AUTH_URNS"`
}

// ComprehensiveData holds all citation data
type ComprehensiveData struct {
	Greek GreekData
	Latin LatinData
	Schol ScholData
	Other OtherData
}

// findDataDir attempts to find the data directory relative to the current working directory
func findDataDir() string {
	// Try current directory first
	if _, err := os.Stat("data"); err == nil {
		return "data"
	}

	// Try parent directories up to 3 levels
	for i := 1; i <= 3; i++ {
		path := strings.Repeat("../", i) + "data"
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Default to "data" if not found
	return "data"
}

// LoadComprehensiveData loads all citation data from JSON files
func LoadComprehensiveData() (*ComprehensiveData, error) {
	data := &ComprehensiveData{}
	dataDir := findDataDir()

	// Load Greek data
	greekBytes, err := os.ReadFile(filepath.Join(dataDir, "greek_data.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read data/greek_data.json: %w", err)
	}
	if err := json.Unmarshal(greekBytes, &data.Greek); err != nil {
		return nil, fmt.Errorf("failed to parse data/greek_data.json: %w", err)
	}

	// Load Latin data
	latinBytes, err := os.ReadFile(filepath.Join(dataDir, "latin_data.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read data/latin_data.json: %w", err)
	}
	if err := json.Unmarshal(latinBytes, &data.Latin); err != nil {
		return nil, fmt.Errorf("failed to parse data/latin_data.json: %w", err)
	}

	// Load Schol data
	scholBytes, err := os.ReadFile(filepath.Join(dataDir, "schol_data.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read data/schol_data.json: %w", err)
	}
	if err := json.Unmarshal(scholBytes, &data.Schol); err != nil {
		return nil, fmt.Errorf("failed to parse data/schol_data.json: %w", err)
	}

	// Load Other data
	otherBytes, err := os.ReadFile(filepath.Join(dataDir, "other_data.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read data/other_data.json: %w", err)
	}
	if err := json.Unmarshal(otherBytes, &data.Other); err != nil {
		return nil, fmt.Errorf("failed to parse data/other_data.json: %w", err)
	}

	// Apply title transformations to generate abbreviations
	data.expandWorkTitles()

	return data, nil
}

// functions to handle Simple vs Range WorkURNs
func (w *WorkURN) IsSimple() bool {
	return w.Range == nil
}

func (w *WorkURN) IsRange() bool {
	return w.Range != nil
}

// this handles polymorphic JSON for WorkURN
func (w *WorkURN) UnmarshalJSON(data []byte) error {
	// first try unmarshalling as simple string
	var simple string
	if err := json.Unmarshal(data, &simple); err == nil {
		w.Simple = simple
		w.Range = nil
		return nil
	}

	// now try to unmarshall as an array in format [prefix, start, end]
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("WorkURN must be either a string or array: %w", err)
	}

	if len(arr) < 3 {
		return fmt.Errorf("WorkURN array must have at least 3 elements in format [prefix, start, end], got %d", len(arr))
	}

	// now WorkURN from JSON known to be in range format
	prefix, ok := arr[0].(string)
	if !ok {
		return fmt.Errorf("WorkURN array[0] must be a string, got %T", arr[0])
	}

	startFloat, ok := arr[1].(float64)
	if !ok {
		return fmt.Errorf("WorkURN array[1] must be a number, got %T", arr[1])
	}

	endFloat, ok := arr[2].(float64)
	if !ok {
		return fmt.Errorf("WorkURN array[2] must be a number, got %T", arr[2])
	}

	w.Range = &WorkRange{
		Prefix: prefix,
		Start:  int(startFloat),
		End:    int(endFloat),
	}

	return nil
}

// expandWorkTitles generates additional abbreviations for work titles
func (cd *ComprehensiveData) expandWorkTitles() {
	// Expand Greek works
	for author, works := range cd.Greek.WorkURNs {
		expanded := make(map[string]WorkURN)
		for work, urn := range works {
			expanded[work] = urn
			// Add abbreviations
			for _, abbrev := range GenerateWorkAbbreviations(work) {
				if _, exists := expanded[abbrev]; !exists {
					expanded[abbrev] = urn
				}
			}
		}
		cd.Greek.WorkURNs[author] = expanded
	}

	// Expand Latin works
	for author, works := range cd.Latin.WorkURNs {
		expanded := make(map[string]WorkURN)
		for work, urn := range works {
			expanded[work] = urn
			// Add abbreviations
			for _, abbrev := range GenerateWorkAbbreviations(work) {
				if _, exists := expanded[abbrev]; !exists {
					expanded[abbrev] = urn
				}
			}
		}
		cd.Latin.WorkURNs[author] = expanded
	}

	// Expand Schol works
	for author, works := range cd.Schol.WorkURNs {
		expanded := make(map[string]WorkURN)
		for work, urn := range works {
			expanded[work] = urn
			// Add abbreviations
			for _, abbrev := range GenerateWorkAbbreviations(work) {
				if _, exists := expanded[abbrev]; !exists {
					expanded[abbrev] = urn
				}
			}
		}
		cd.Schol.WorkURNs[author] = expanded
	}

	// Expand Other works
	for author, works := range cd.Other.WorkURNs {
		expanded := make(map[string]WorkURN)
		for work, urn := range works {
			expanded[work] = urn
			// Add abbreviations
			for _, abbrev := range GenerateWorkAbbreviations(work) {
				if _, exists := expanded[abbrev]; !exists {
					expanded[abbrev] = urn
				}
			}
		}
		cd.Other.WorkURNs[author] = expanded
	}
}

// GetAllAuthors returns a set of all known authors
func (cd *ComprehensiveData) GetAllAuthors() map[string]bool {
	authors := make(map[string]bool)

	for author := range cd.Greek.AuthURNs {
		authors[author] = true
	}
	for author := range cd.Latin.AuthURNs {
		authors[author] = true
	}
	for author := range cd.Schol.AuthURNs {
		authors[author] = true
	}
	for author := range cd.Other.AuthURNs {
		authors[author] = true
	}

	return authors
}

// GetAllAuthAbb returns a combined map of all author abbreviations
func (cd *ComprehensiveData) GetAllAuthAbb() map[string]interface{} {
	combined := make(map[string]interface{})

	// Add Greek abbreviations
	for abbrev, author := range cd.Greek.AuthAbb {
		combined[abbrev] = author
	}

	// Add Latin abbreviations (can include functions)
	for abbrev, value := range cd.Latin.AuthAbb {
		combined[abbrev] = value
	}

	// Add Schol abbreviations
	for abbrev, author := range cd.Schol.AuthAbb {
		combined[abbrev] = author
	}

	// Add Other abbreviations
	for abbrev, author := range cd.Other.AuthAbb {
		combined[abbrev] = author
	}

	return combined
}

// GetAllWorkURNs returns a combined map of all work URNs
func (cd *ComprehensiveData) GetAllWorkURNs() map[string]map[string]WorkURN {
	combined := make(map[string]map[string]WorkURN)

	// Add Greek works
	for author, works := range cd.Greek.WorkURNs {
		combined[author] = works
	}

	// Add Latin works
	for author, works := range cd.Latin.WorkURNs {
		combined[author] = works
	}

	// Add Schol works
	for author, works := range cd.Schol.WorkURNs {
		combined[author] = works
	}

	// Add Other works
	for author, works := range cd.Other.WorkURNs {
		combined[author] = works
	}

	return combined
}

// GetAllAuthURNs returns a combined map of all author URNs
func (cd *ComprehensiveData) GetAllAuthURNs() map[string]string {
	combined := make(map[string]string)

	// Add Greek URNs
	for author, urn := range cd.Greek.AuthURNs {
		combined[author] = urn
	}

	// Add Latin URNs
	for author, urn := range cd.Latin.AuthURNs {
		combined[author] = urn
	}

	// Add Schol URNs
	for author, urn := range cd.Schol.AuthURNs {
		combined[author] = urn
	}

	// Add Other URNs
	for author, urn := range cd.Other.AuthURNs {
		combined[author] = urn
	}

	return combined
}

// IsSingleWorkAuthor checks if an author is known for a single work
func (cd *ComprehensiveData) IsSingleWorkAuthor(author string) bool {
	// Check Greek single work authors
	for _, swa := range cd.Greek.SingleWorkAuthors {
		if swa == author {
			return true
		}
	}

	// Check Latin single work authors
	for _, swa := range cd.Latin.SingleWorkAuthors {
		if swa == author {
			return true
		}
	}

	return false
}

// ResolveLatinAuthorFunction handles special Latin author disambiguation
func (cd *ComprehensiveData) ResolveLatinAuthorFunction(abbrev, work string) string {
	value, exists := cd.Latin.AuthAbb[abbrev]
	if !exists {
		return ""
	}

	// Handle function references for Pliny and Seneca
	if valueStr, ok := value.(string); ok {
		// If it's a function reference (like "_which_pliny"), don't return it directly
		// but fall through to the switch statement below
		if valueStr != "_which_pliny" && valueStr != "_which_seneca" {
			return valueStr
		}
	}

	// Handle function cases based on work title
	switch abbrev {
	case "plin.", "pliny":
		// Check which Pliny based on work
		work = strings.ToLower(work)

		// Check pliny_senior works (exact match and abbreviations)
		if cd.Latin.WorkURNs["pliny_senior"] != nil {
			if _, exists := cd.Latin.WorkURNs["pliny_senior"][work]; exists {
				return "pliny_senior"
			}
			// Check abbreviations
			for title := range cd.Latin.WorkURNs["pliny_senior"] {
				abbrevs := GenerateWorkAbbreviations(title)
				for _, abbrev := range abbrevs {
					if abbrev == work {
						return "pliny_senior"
					}
				}
			}
		}

		// Check pliny_junior works (exact match and abbreviations)
		if cd.Latin.WorkURNs["pliny_junior"] != nil {
			if _, exists := cd.Latin.WorkURNs["pliny_junior"][work]; exists {
				return "pliny_junior"
			}
			// Check abbreviations
			for title := range cd.Latin.WorkURNs["pliny_junior"] {
				abbrevs := GenerateWorkAbbreviations(title)
				for _, abbrev := range abbrevs {
					if abbrev == work {
						return "pliny_junior"
					}
				}
			}
		}

		return "pliny_senior" // default

	case "sen.", "seneca":
		// Check which Seneca based on work
		work = strings.ToLower(work)

		// Check seneca_senior works (exact match and abbreviations)
		if cd.Latin.WorkURNs["seneca_senior"] != nil {
			if _, exists := cd.Latin.WorkURNs["seneca_senior"][work]; exists {
				return "seneca_senior"
			}
			// Check abbreviations
			for title := range cd.Latin.WorkURNs["seneca_senior"] {
				abbrevs := GenerateWorkAbbreviations(title)
				for _, abbrev := range abbrevs {
					if abbrev == work {
						return "seneca_senior"
					}
				}
			}
		}

		// Check seneca_junior works (exact match and abbreviations)
		if cd.Latin.WorkURNs["seneca_junior"] != nil {
			if _, exists := cd.Latin.WorkURNs["seneca_junior"][work]; exists {
				return "seneca_junior"
			}
			// Check abbreviations
			for title := range cd.Latin.WorkURNs["seneca_junior"] {
				abbrevs := GenerateWorkAbbreviations(title)
				for _, abbrev := range abbrevs {
					if abbrev == work {
						return "seneca_junior"
					}
				}
			}
		}

		return "seneca_junior" // default
	}

	return ""
}

func GenerateWorkAbbreviations(title string) []string {
	var abbreviations []string
	title = strings.ToLower(title)

	// Skip if numeric
	if regexp.MustCompile(`^\d+$`).MatchString(title) {
		return abbreviations
	}

	words := strings.Fields(title)
	if len(words) == 0 {
		return abbreviations
	}

	// Track existing abbreviations to avoid duplicates
	existing := make(map[string]bool)

	// Helper function to add unique abbreviations
	addAbbrev := func(abbrev string) {
		if abbrev != "" && !existing[abbrev] {
			abbreviations = append(abbreviations, abbrev)
			existing[abbrev] = true
		}
	}

	// Latinize and anglicize one-word plural titles (from Python)
	if len(words) == 1 {
		if strings.HasSuffix(title, "s") {
			addAbbrev(title[:len(title)-1] + "a")
		} else if strings.HasSuffix(title, "a") {
			addAbbrev(title[:len(title)-1] + "s")
		}
	}

	// First letter variations
	addAbbrev(string(title[0]))
	addAbbrev(string(title[0]) + ".")

	// First N letters (2-6) from first word
	for i := 2; i <= 6 && i <= len(words[0]); i++ {
		abbrev := words[0][:i]
		addAbbrev(abbrev)
		addAbbrev(abbrev + ".")
	}

	// Initials - multiple variations (from Python)
	if len(words) > 1 {
		// Standard initials
		initials := ""
		dotInitials := ""
		underInitials := ""
		dotUnderInitials := ""

		for _, word := range words {
			if len(word) > 0 {
				initials += string(word[0])
				dotInitials += string(word[0]) + "."
				underInitials += string(word[0]) + "_"
				dotUnderInitials += string(word[0]) + "._"
			}
		}

		addAbbrev(initials)
		addAbbrev(dotInitials)
		addAbbrev(strings.TrimSuffix(underInitials, "_"))
		addAbbrev(strings.TrimSuffix(dotUnderInitials, "._") + ".")

		// Function words to skip (from Python)
		funcWords := map[string]bool{
			"the": true, "a": true, "an": true, "of": true, "in": true,
			"by": true, "for": true, "on": true, "and": true, "de": true, "ad": true,
		}

		// Initials without function words
		hasFuncWords := false
		for _, word := range words {
			if funcWords[word] {
				hasFuncWords = true
				break
			}
		}

		if hasFuncWords {
			initials = ""
			dotInitials = ""
			underInitials = ""
			dotUnderInitials = ""

			for _, word := range words {
				if !funcWords[word] && len(word) > 0 {
					initials += string(word[0])
					dotInitials += string(word[0]) + "."
					underInitials += string(word[0]) + "_"
					dotUnderInitials += string(word[0]) + "._"
				}
			}

			addAbbrev(initials)
			addAbbrev(dotInitials)
			addAbbrev(strings.TrimSuffix(underInitials, "_"))
			addAbbrev(strings.TrimSuffix(dotUnderInitials, "._") + ".")
		}

		// First word only
		addAbbrev(words[0])
	}

	// Multi-word combinations
	if len(words) > 2 {
		twoWords := strings.Join(words[:2], " ")
		addAbbrev(twoWords)
		addAbbrev(strings.ReplaceAll(twoWords, " ", "_"))
	}

	if len(words) > 3 {
		threeWords := strings.Join(words[:3], " ")
		addAbbrev(threeWords)
		addAbbrev(strings.ReplaceAll(threeWords, " ", "_"))
	}

	// Smart suspension (simplified version of Python's linguistic rules)
	smartSusp := generateSmartSuspension(title, true) // skip "de"
	addAbbrev(smartSusp)

	smartSuspNoDe := generateSmartSuspension(title, false) // don't skip "de"
	addAbbrev(smartSuspNoDe)

	// Underscored version
	if len(words) > 1 {
		underscored := strings.ReplaceAll(title, " ", "_")
		addAbbrev(underscored)
	}

	// Add the full title itself (this was missing!)
	addAbbrev(title)

	return abbreviations
}

// generateSmartSuspension implements a simplified version of Python's smart suspension logic
func generateSmartSuspension(title string, skipDe bool) string {
	funcWords := map[string]bool{
		"the": true, "a": true, "an": true, "of": true, "in": true,
		"by": true, "for": true, "on": true, "and": true, "de": true, "ad": true,
	}
	vowels := map[rune]bool{'a': true, 'e': true, 'i': true, 'o': true, 'u': true, 'y': true}
	plosives := map[rune]bool{'t': true, 'p': true, 'd': true, 'g': true, 'k': true, 'x': true, 'c': true, 'b': true}

	words := strings.Fields(strings.ReplaceAll(title, " ", "_"))
	var suspensions []string

	for _, word := range words {
		if word == "de" || word == "on" {
			if skipDe {
				continue
			}
			suspensions = append(suspensions, word)
			continue
		}

		if funcWords[word] {
			continue
		}

		var buffer []rune
		vowelSeen := false
		consLastSeen := false

		for _, char := range word {
			if vowels[char] {
				if vowelSeen && consLastSeen {
					break
				}
				vowelSeen = true
				consLastSeen = false
				buffer = append(buffer, char)
			} else {
				if vowelSeen && plosives[char] {
					buffer = append(buffer, char)
					break
				} else {
					consLastSeen = true
					buffer = append(buffer, char)
				}
			}
		}

		if len(buffer) > 0 {
			suspensions = append(suspensions, string(buffer))
		}
	}

	if len(suspensions) == 0 {
		return ""
	}

	result := strings.Join(suspensions, ".")
	if !strings.HasSuffix(result, ".") {
		result += "."
	}

	return result
}
