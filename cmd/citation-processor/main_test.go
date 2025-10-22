package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"perseus_citation_linker/pkg/resolver"
	"perseus_citation_linker/pkg/loader"
)

// Citation struct is imported from main.go

// findTestDataDir attempts to find the testdata directory
func findTestDataDir() string {
	// Try current directory first
	if _, err := os.Stat("testdata"); err == nil {
		return "testdata"
	}

	// Try parent directories up to 3 levels
	for i := 1; i <= 3; i++ {
		path := strings.Repeat("../", i) + "testdata"
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Default to "testdata" if not found
	return "testdata"
}

// TestCitationProcessing runs end-to-end tests on XML files and compares output
func TestCitationProcessing(t *testing.T) {
	testDataDir := findTestDataDir()

	// Clean and create output directory in testdata
	testOutputDir := filepath.Join(testDataDir, "output")
	os.RemoveAll(testOutputDir)
	err := os.MkdirAll(testOutputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create testdata/output directory: %v", err)
	}

	tests := []struct {
		name     string
		xmlFile  string
		useCitTags bool
		expected struct {
			resolvedFile   string
			unresolvedFile string
		}
	}{
		{
			name:       "campbell-sophlanguage-2-bibl-mode",
			xmlFile:    filepath.Join(testDataDir, "xml/campbell-sophlanguage-2.xml"),
			useCitTags: false,
			expected: struct {
				resolvedFile   string
				unresolvedFile string
			}{
				resolvedFile:   filepath.Join(testDataDir, "expected/campbell-sophlanguage-2_resolved.jsonl"),
				unresolvedFile: filepath.Join(testDataDir, "expected/campbell-sophlanguage-2_unresolved.jsonl"),
			},
		},
		{
			name:       "viaf-cit-mode",
			xmlFile:    filepath.Join(testDataDir, "xml/viaf2603144.viaf001.perseus-eng1.xml"),
			useCitTags: true,
			expected: struct {
				resolvedFile   string
				unresolvedFile string
			}{
				resolvedFile:   filepath.Join(testDataDir, "expected/viaf2603144_resolved.jsonl"),
				unresolvedFile: filepath.Join(testDataDir, "expected/viaf2603144_unresolved.jsonl"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use persistent output directory in testdata/output
			outputDir := filepath.Join(testOutputDir, tt.name)
			err := os.MkdirAll(outputDir, 0755)
			if err != nil {
				t.Fatalf("Failed to create test output directory: %v", err)
			}

			resolvedFile := filepath.Join(outputDir, "resolved.jsonl")
			unresolvedFile := filepath.Join(outputDir, "unresolved.jsonl")

			// Process the XML file
			config := Config{
				InputDir:       "testdata/xml",
				OutputDir:      outputDir,
				ResolvedFile:   "resolved.jsonl",
				UnresolvedFile: "unresolved.jsonl",
				UseCitTags:     tt.useCitTags,
			}

			processor, err := NewCitationProcessor(config)
			if err != nil {
				t.Fatalf("Failed to create citation processor: %v", err)
			}

			// Process just the specific XML file
			err = processor.ProcessXMLFile(tt.xmlFile)
			if err != nil {
				t.Fatalf("Failed to process XML file %s: %v", tt.xmlFile, err)
			}

			// Compare resolved citations
			t.Run("resolved_citations", func(t *testing.T) {
				expectedResolved, err := loadCitations(tt.expected.resolvedFile)
				if err != nil {
					t.Fatalf("Failed to load expected resolved citations: %v", err)
				}

				actualResolved, err := loadCitations(resolvedFile)
				if err != nil {
					t.Fatalf("Failed to load actual resolved citations: %v", err)
				}

				compareCitations(t, "resolved", expectedResolved, actualResolved)
			})

			// Compare unresolved citations
			t.Run("unresolved_citations", func(t *testing.T) {
				expectedUnresolved, err := loadCitations(tt.expected.unresolvedFile)
				if err != nil {
					t.Fatalf("Failed to load expected unresolved citations: %v", err)
				}

				actualUnresolved, err := loadCitations(unresolvedFile)
				if err != nil {
					t.Fatalf("Failed to load actual unresolved citations: %v", err)
				}

				compareCitations(t, "unresolved", expectedUnresolved, actualUnresolved)
			})
		})
	}
}

// TestCitationResolver tests individual resolver functions
func TestCitationResolver(t *testing.T) {
	urnResolver, err := resolver.NewURNResolver()
	if err != nil {
		t.Fatalf("Failed to create URN resolver: %v", err)
	}
	_ = urnResolver // Mark as used to avoid unused variable error

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Shakespeare Cymbeline",
			input:    "shakespeare cymb. iv. 2",
			expected: "urn:cts:englishLit:shak.cym.perseus-eng2:iv.2",
		},
		{
			name:     "Sophocles Electra",
			input:    "soph. el. 123",
			expected: "urn:cts:greekLit:tlg0011.tlg005.perseus-grc2:123",
		},
		{
			name:     "Simple Greek citation",
			input:    "hom. il. 1.1",
			expected: "urn:cts:greekLit:tlg0012.tlg001.perseus-grc2:1.1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := urnResolver.GetURN(tc.input, "", "test")
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

// TestWorkAbbreviations tests work abbreviation generation
func TestWorkAbbreviations(t *testing.T) {
	testCases := []struct {
		title    string
		expected []string
	}{
		{
			title:    "cymbeline",
			expected: []string{"c", "c.", "cy", "cy.", "cym", "cym.", "cymb", "cymb.", "cymbeline"},
		},
		{
			title:    "electra",
			expected: []string{"e", "e.", "el", "el.", "ele", "ele.", "elec", "elec.", "elect", "elect.", "electra"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			result := loader.GenerateWorkAbbreviations(tc.title)

			// Check that all expected abbreviations are present
			for _, expected := range tc.expected {
				found := false
				for _, abbrev := range result {
					if abbrev == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected abbreviation '%s' not found in %v", expected, result)
				}
			}
		})
	}
}

// loadCitations loads citations from a JSONL file
func loadCitations(filename string) ([]Citation, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var citations []Citation
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var citation Citation
		if err := json.Unmarshal([]byte(line), &citation); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line: %w", err)
		}
		citations = append(citations, citation)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return citations, nil
}

// compareCitations compares two slices of citations for equality
func compareCitations(t *testing.T, citationType string, expected, actual []Citation) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("%s citations count mismatch: expected %d, got %d",
			citationType, len(expected), len(actual))
		return
	}

	// Sort both slices by a consistent key for comparison
	sortCitations(expected)
	sortCitations(actual)

	// Normalize filenames before comparison
	normalizeFilenames(expected)
	normalizeFilenames(actual)

	// Compare each citation
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("%s citation %d mismatch:\nExpected: %+v\nActual: %+v",
				citationType, i, expected[i], actual[i])
		}
	}

	t.Logf("%s citations: %d entries match perfectly", citationType, len(expected))
}

// normalizeFilenames normalizes filenames to just the base name for comparison
func normalizeFilenames(citations []Citation) {
	for i := range citations {
		if citations[i].Filename != "" {
			citations[i].Filename = filepath.Base(citations[i].Filename)
		}
	}
}

// sortCitations sorts citations by a consistent key for deterministic comparison
func sortCitations(citations []Citation) {
	sort.Slice(citations, func(i, j int) bool {
		// Primary sort: by bibl content
		if citations[i].Bibl != citations[j].Bibl {
			return citations[i].Bibl < citations[j].Bibl
		}
		// Secondary sort: by doc_cit_urn
		return citations[i].DocCitURN < citations[j].DocCitURN
	})
}

// TestCustomDataDir tests using a custom data directory
// This test creates a temporary data directory, copies the required JSON files,
// tests the custom directory functionality, and cleans up automatically
func TestCustomDataDir(t *testing.T) {
	// Create temporary directory for custom data files
	tempDataDir := t.TempDir()

	// Find the actual data directory to copy files from
	var actualDataDir string
	if _, err := os.Stat("../../data"); err == nil {
		actualDataDir = "../../data"
	} else if _, err := os.Stat("data"); err == nil {
		actualDataDir = "data"
	} else if _, err := os.Stat("../data"); err == nil {
		actualDataDir = "../data"
	} else {
		t.Fatal("Could not find data directory to copy test files from")
	}

	// Copy all required JSON files to temp directory
	requiredFiles := []string{
		"greek_data.json",
		"latin_data.json",
		"schol_data.json",
		"other_data.json",
	}

	for _, filename := range requiredFiles {
		sourcePath := filepath.Join(actualDataDir, filename)
		destPath := filepath.Join(tempDataDir, filename)

		// Read source file
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", sourcePath, err)
		}

		// Write to temp directory
		err = os.WriteFile(destPath, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write %s: %v", destPath, err)
		}
	}

	// Test 1: LoadComprehensiveDataDir with custom directory
	t.Run("LoadComprehensiveDataDir", func(t *testing.T) {
		data, err := loader.LoadComprehensiveDataDir(tempDataDir)
		if err != nil {
			t.Fatalf("Failed to load data from custom directory: %v", err)
		}

		// Verify data was loaded correctly
		if len(data.Greek.AuthURNs) == 0 {
			t.Error("Greek author URNs not loaded")
		}
		if len(data.Latin.AuthURNs) == 0 {
			t.Error("Latin author URNs not loaded")
		}
		if len(data.Schol.AuthURNs) == 0 {
			t.Error("Schol author URNs not loaded")
		}
		if len(data.Other.AuthURNs) == 0 {
			t.Error("Other author URNs not loaded")
		}

		t.Logf("Successfully loaded data from custom directory %s", tempDataDir)
		t.Logf("  Greek authors: %d", len(data.Greek.AuthURNs))
		t.Logf("  Latin authors: %d", len(data.Latin.AuthURNs))
		t.Logf("  Schol authors: %d", len(data.Schol.AuthURNs))
		t.Logf("  Other authors: %d", len(data.Other.AuthURNs))
	})

	// Test 2: NewURNResolverFromDir with custom directory
	t.Run("NewURNResolverFromDir", func(t *testing.T) {
		resolver, err := resolver.NewURNResolverFromDir(tempDataDir)
		if err != nil {
			t.Fatalf("Failed to create resolver from custom directory: %v", err)
		}

		// Test that resolver works with loaded data
		testCases := []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "Sophocles Electra",
				input:    "soph. el. 123",
				expected: "urn:cts:greekLit:tlg0011.tlg005.perseus-grc2:123",
			},
			{
				name:     "Homer Iliad",
				input:    "hom. il. 1.1",
				expected: "urn:cts:greekLit:tlg0012.tlg001.perseus-grc2:1.1",
			},
			{
				name:     "Pliny Senior",
				input:    "plin. nat. hist. 15.30",
				expected: "urn:cts:latinLit:phi0978.phi001.perseus-lat2:15.30",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := resolver.GetURN(tc.input, "", "test")
				if result != tc.expected {
					t.Errorf("Expected %s, got %s", tc.expected, result)
				}
			})
		}

		t.Logf("Successfully created and tested resolver with custom data directory")
	})

	// Test 3: Verify temp directory cleanup (automatic with t.TempDir())
	t.Run("VerifyTempDirExists", func(t *testing.T) {
		// Verify temp directory still exists during test
		if _, err := os.Stat(tempDataDir); os.IsNotExist(err) {
			t.Error("Temp directory was cleaned up too early")
		}
		t.Logf("Temp directory %s exists and will be cleaned up after test", tempDataDir)
	})

	// Note: t.TempDir() automatically cleans up the temp directory after test completes
}

// BenchmarkCitationProcessing benchmarks the citation processing performance
func BenchmarkCitationProcessing(b *testing.B) {
	testDataDir := findTestDataDir()
	tempDir := b.TempDir()
	config := Config{
		InputDir:       filepath.Join(testDataDir, "xml"),
		OutputDir:      tempDir,
		ResolvedFile:   "resolved.jsonl",
		UnresolvedFile: "unresolved.jsonl",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		processor, err := NewCitationProcessor(config)
		if err != nil {
			b.Fatalf("Failed to create citation processor: %v", err)
		}

		err = processor.ProcessXMLFile(filepath.Join(testDataDir, "xml/campbell-sophlanguage-2.xml"))
		if err != nil {
			b.Fatalf("Failed to process XML file: %v", err)
		}
	}
}