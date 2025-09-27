package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"perseus_citation_linker/pkg/resolver"
)

type Citation struct {
	NAttrib    string `json:"n_attrib"`
	Bibl       string `json:"bibl"`
	Ref        string `json:"ref"`
	URN        string `json:"urn"`
	Quote      string `json:"quote"`
	XMLContext string `json:"xml_context"`
	Filename   string `json:"filename"`
	DocCitURN  string `json:"doc_cit_urn"`
}

type Config struct {
	InputDir       string
	OutputDir      string
	ResolvedFile   string
	UnresolvedFile string
	UseCitTags     bool
}

type CitationProcessor struct {
	Config      Config
	Resolver    *resolver.URNResolver
	Counter     int
	CounterMux  sync.Mutex
}

func NewCitationProcessor(config Config) (*CitationProcessor, error) {
	urnResolver, err := resolver.NewURNResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	return &CitationProcessor{
		Config:   config,
		Resolver: urnResolver,
		Counter:  0,
	}, nil
}

func main() {
	// Parse command line flags
	var useCitTags = flag.Bool("cit", false, "Use <cit> tags to guide citation extraction (default: use <bibl> tags only)")
	var inputDir = flag.String("input", ".", "Input directory containing XML files")
	var outputDir = flag.String("output", "cit_data", "Output directory for JSONL files")
	flag.Parse()

	config := Config{
		InputDir:       *inputDir,
		OutputDir:      *outputDir,
		ResolvedFile:   "resolved.jsonl",
		UnresolvedFile: "unresolved.jsonl",
		UseCitTags:     *useCitTags,
	}

	processor, err := NewCitationProcessor(config)
	if err != nil {
		log.Fatalf("Error creating processor: %v", err)
	}

	if err := processor.ProcessAllXMLFiles(); err != nil {
		log.Fatalf("Error processing files: %v", err)
	}

	fmt.Println("Citation processing completed successfully")
}

func (cp *CitationProcessor) ProcessAllXMLFiles() error {

	// Create output directory
	if err := os.MkdirAll(cp.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Clean existing output files
	resolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.ResolvedFile)
	unresolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.UnresolvedFile)

	os.Remove(resolvedPath)
	os.Remove(unresolvedPath)

	// Find all XML files in the input directory
	pattern := filepath.Join(cp.Config.InputDir, "*.xml")
	xmlFiles, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("error finding XML files: %w", err)
	}
	for _, xmlFile := range xmlFiles {
		fmt.Printf("Processing %s...\n", xmlFile)
		if err := cp.ProcessXMLFile(xmlFile); err != nil {
			log.Printf("Error processing %s: %v", xmlFile, err)
			continue
		}
	}

	return nil
}

func (cp *CitationProcessor) ProcessXMLFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Extract citations from XML content
	citations := cp.ExtractCitations(string(content), filename)

	// Write citations to appropriate output files
	return cp.WriteCitations(citations)
}

func (cp *CitationProcessor) ExtractCitations(xmlContent, filename string) []Citation {
	var allCitations []Citation

	if cp.Config.UseCitTags {
		// Comprehensive extraction approach - find all citation patterns regardless of XML structure
		allCitations = cp.extractAllCitationPatterns(xmlContent, filename)
	} else {
		// Original behavior: only extract <bibl> tags
		allCitations = cp.extractBiblTags(xmlContent, filename)
	}

	return allCitations
}

// extractBiblTags extracts citations using <bibl> tags directly (original method)
func (cp *CitationProcessor) extractBiblTags(xmlContent, filename string) []Citation {
	// Regex to find <bibl> elements
	biblRegex := regexp.MustCompile(`<bibl[^>]*>.*?</bibl>`)
	matches := biblRegex.FindAllStringSubmatch(xmlContent, -1)

	var citations []Citation

	for _, match := range matches {
		if len(match) > 0 {
			citation := cp.ProcessCitation(match[0], xmlContent, filename)
			citations = append(citations, citation)
		}
	}

	return citations
}

// extractCitationsTags extracts citations using <cit> containers with <bibl> and <quote> elements
func (cp *CitationProcessor) extractCitationsTags(xmlContent, filename string) []Citation {
	// Regex to find <cit> elements - match Python pattern exactly
	// Python uses: r"<cit.+?/cit>" which matches both <cit>...</cit> and potential self-closing variants
	citRegex := regexp.MustCompile(`(?s)<cit.+?/cit>`)
	citMatches := citRegex.FindAllString(xmlContent, -1)

	var citations []Citation
	for _, citMatch := range citMatches {
		citation := cp.processCitationTag(citMatch, xmlContent, filename)
		if citation.Bibl != "" { // Only add if we found a bibl element
			citations = append(citations, citation)
		}
	}
	return citations
}

// extractStandaloneBiblTags extracts <bibl> tags that are NOT within <cit> containers
func (cp *CitationProcessor) extractStandaloneBiblTags(xmlContent, filename string) []Citation {
	// First, remove all <cit> containers to avoid double-counting
	// Use same pattern as extractCitationsTags
	citRegex := regexp.MustCompile(`(?s)<cit.+?/cit>`)
	contentWithoutCit := citRegex.ReplaceAllString(xmlContent, "")

	// Now extract <bibl> elements from the remaining content
	biblRegex := regexp.MustCompile(`<bibl[^>]*>.*?</bibl>`)
	matches := biblRegex.FindAllStringSubmatch(contentWithoutCit, -1)

	var citations []Citation

	for _, match := range matches {
		if len(match) > 0 {
			citation := cp.ProcessCitation(match[0], xmlContent, filename)
			citations = append(citations, citation)
		}
	}

	return citations
}

// processCitationTag processes a single <cit> element containing <bibl> and <quote>
func (cp *CitationProcessor) processCitationTag(citMatch, xmlContent, filename string) Citation {
	cp.CounterMux.Lock()
	cp.Counter++
	citURN := fmt.Sprintf(":citations-%d.%d", 1, cp.Counter)
	cp.CounterMux.Unlock()

	// Extract bibl element from within the cit tag
	biblRegex := regexp.MustCompile(`<bibl[^>]*>.*?</bibl>`)
	biblMatch := biblRegex.FindString(citMatch)

	if biblMatch == "" {
		// No bibl found in this cit element
		return Citation{}
	}

	// Extract quote element from within the cit tag
	quoteRegex := regexp.MustCompile(`(?s)<quote[^>]*>(.*?)</quote>`)
	quoteMatches := quoteRegex.FindStringSubmatch(citMatch)
	var quote string
	if len(quoteMatches) > 1 {
		quote = strings.TrimSpace(quoteMatches[1])
	}

	// Extract n attribute from bibl tag
	nAttr := cp.extractAttribute(biblMatch, "n")

	// Extract bibl content (text between tags)
	biblContent := cp.extractBiblContent(biblMatch)

	// Get reference string for URN resolution
	ref := cp.Resolver.GetRef(nAttr, biblContent)

	// Resolve to URN
	var urn string
	if ref != "" {
		urn = cp.Resolver.GetURN(ref, citMatch, filename)
	}

	// Extract context around the citation
	context := cp.extractContext(xmlContent, citMatch, 500)

	return Citation{
		NAttrib:    nAttr,
		Bibl:       biblContent,
		Ref:        ref,
		URN:        urn,
		Quote:      quote,
		XMLContext: context,
		Filename:   filename,
		DocCitURN:  citURN,
	}
}

func (cp *CitationProcessor) ProcessCitation(biblMatch, xmlContent, filename string) Citation {
	cp.CounterMux.Lock()
	cp.Counter++
	citURN := fmt.Sprintf(":citations-%d.%d", 1, cp.Counter) // Simplified URN structure
	cp.CounterMux.Unlock()

	// Extract n attribute
	nAttr := cp.extractAttribute(biblMatch, "n")

	// Extract bibl content
	biblContent := cp.extractBiblContent(biblMatch)

	// Extract quote (look for quote element after bibl)
	quote := cp.extractQuote(xmlContent, biblMatch)

	// Extract context (500 chars before and after)
	context := cp.extractContext(xmlContent, biblMatch, 500)

	// Get standardized reference
	ref := cp.Resolver.GetRef(nAttr, biblContent)

	// Resolve to URN
	urn := ""
	if ref != "" {
		urn = cp.Resolver.GetURN(ref, context, filename)
	}

	return Citation{
		NAttrib:    nAttr,
		Bibl:       biblContent,
		Ref:        ref,
		URN:        urn,
		Quote:      quote,
		XMLContext: context,
		Filename:   filename,
		DocCitURN:  citURN,
	}
}

func (cp *CitationProcessor) extractAttribute(element, attrName string) string {
	pattern := fmt.Sprintf(`%s="([^"]*)"`, attrName)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(element)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func (cp *CitationProcessor) extractBiblContent(biblElement string) string {
	re := regexp.MustCompile(`<bibl[^>]*>(.*?)</bibl>`)
	match := re.FindStringSubmatch(biblElement)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func (cp *CitationProcessor) extractQuote(xmlContent, biblMatch string) string {
	// Find position of bibl match in content
	index := strings.Index(xmlContent, biblMatch)
	if index == -1 {
		return ""
	}

	// Look for quote element after bibl
	afterBibl := xmlContent[index+len(biblMatch):]
	quoteRegex := regexp.MustCompile(`<quote[^>]*>(.*?)</quote>`)
	match := quoteRegex.FindStringSubmatch(afterBibl[:min(len(afterBibl), 200)])

	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func (cp *CitationProcessor) extractContext(xmlContent, biblMatch string, contextSize int) string {
	index := strings.Index(xmlContent, biblMatch)
	if index == -1 {
		return ""
	}

	start := max(0, index-contextSize)
	end := min(len(xmlContent), index+len(biblMatch)+contextSize)

	context := xmlContent[start:end]
	// Clean up whitespace
	context = regexp.MustCompile(`\s+`).ReplaceAllString(context, " ")
	return strings.TrimSpace(context)
}

func (cp *CitationProcessor) WriteCitations(citations []Citation) error {
	resolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.ResolvedFile)
	unresolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.UnresolvedFile)

	resolvedFile, err := os.OpenFile(resolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer resolvedFile.Close()

	unresolvedFile, err := os.OpenFile(unresolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer unresolvedFile.Close()

	for _, citation := range citations {
		jsonData, err := json.Marshal(citation)
		if err != nil {
			continue
		}

		if citation.URN != "" && citation.Ref != "" {
			// Successfully resolved
			resolvedFile.Write(jsonData)
			resolvedFile.WriteString("\n")
		} else {
			// Failed to resolve
			unresolvedFile.Write(jsonData)
			unresolvedFile.WriteString("\n")
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractAllCitationPatterns finds all citation patterns in any XML structure
// This is a comprehensive approach that doesn't depend on specific XML hierarchy
func (cp *CitationProcessor) extractAllCitationPatterns(xmlContent, filename string) []Citation {
	var allCitations []Citation
	citationMap := make(map[string]bool) // To avoid duplicates

	// Pattern 1: Extract ALL <cit> elements anywhere in the document
	citRegex := regexp.MustCompile(`(?s)<cit\b[^>]*>.*?</cit>`)
	citMatches := citRegex.FindAllString(xmlContent, -1)

	for _, citMatch := range citMatches {
		citation := cp.processCitationTag(citMatch, xmlContent, filename)
		if citation.Bibl != "" {
			key := citation.Bibl + "|" + citation.NAttrib + "|" + citation.Quote
			if !citationMap[key] {
				allCitations = append(allCitations, citation)
				citationMap[key] = true
			}
		}
	}

	// Pattern 2: Extract ALL standalone <bibl> elements (not within <cit>)
	// First remove all <cit> containers to avoid double-counting
	contentWithoutCit := citRegex.ReplaceAllString(xmlContent, "")
	biblRegex := regexp.MustCompile(`<bibl\b[^>]*>.*?</bibl>`)
	biblMatches := biblRegex.FindAllString(contentWithoutCit, -1)

	for _, biblMatch := range biblMatches {
		citation := cp.ProcessCitation(biblMatch, xmlContent, filename)
		if citation.Bibl != "" {
			key := citation.Bibl + "|" + citation.NAttrib + "|" + citation.Quote
			if !citationMap[key] {
				allCitations = append(allCitations, citation)
				citationMap[key] = true
			}
		}
	}

	// Pattern 3: Look for <bibl> elements with n attributes that might have quotes nearby
	// This catches cases where bibl and quote might not be in a formal <cit> structure
	biblWithNRegex := regexp.MustCompile(`<bibl\b[^>]*\bn\s*=\s*"([^"]+)"[^>]*>([^<]*)</bibl>`)
	biblWithNMatches := biblWithNRegex.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range biblWithNMatches {
		if len(match) >= 3 {
			nAttr := match[1]
			biblContent := strings.TrimSpace(match[2])

			// Look for nearby quote elements (within 500 characters)
			biblIndex := strings.Index(xmlContent, match[0])
			if biblIndex >= 0 {
				start := max(0, biblIndex-250)
				end := min(len(xmlContent), biblIndex+len(match[0])+250)
				context := xmlContent[start:end]

				quoteRegex := regexp.MustCompile(`<quote[^>]*>([^<]+)</quote>`)
				quoteMatches := quoteRegex.FindAllStringSubmatch(context, -1)

				var quote string
				if len(quoteMatches) > 0 && len(quoteMatches[0]) > 1 {
					quote = strings.TrimSpace(quoteMatches[0][1])
				}

				citation := cp.createCitationFromParts(nAttr, biblContent, quote, xmlContent, filename)
				if citation.Bibl != "" {
					key := citation.Bibl + "|" + citation.NAttrib + "|" + citation.Quote
					if !citationMap[key] {
						allCitations = append(allCitations, citation)
						citationMap[key] = true
					}
				}
			}
		}
	}

	// Pattern 4: Look for <ref> elements that might contain citations
	// Be more selective - only include if they resolve to valid URNs
	refRegex := regexp.MustCompile(`<ref\b[^>]*>([^<]+)</ref>`)
	refMatches := refRegex.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range refMatches {
		if len(match) >= 2 {
			refContent := strings.TrimSpace(match[1])
			// Only consider ref content that looks like a real citation (has author.work pattern)
			if refContent != "" && regexp.MustCompile(`[A-Za-z]+\.\s*[A-Za-z]*\s*\d+`).MatchString(refContent) {
				citation := cp.createCitationFromParts("", refContent, "", xmlContent, filename)
				if citation.Bibl != "" && citation.URN != "" {
					key := citation.Bibl + "|" + citation.NAttrib + "|" + citation.Quote
					if !citationMap[key] {
						allCitations = append(allCitations, citation)
						citationMap[key] = true
					}
				}
			}
		}
	}


	return allCitations
}

// createCitationFromParts creates a Citation from individual components
func (cp *CitationProcessor) createCitationFromParts(nAttr, biblContent, quote, xmlContent, filename string) Citation {
	cp.CounterMux.Lock()
	cp.Counter++
	citURN := fmt.Sprintf(":citations-%d.%d", 1, cp.Counter)
	cp.CounterMux.Unlock()

	// Get reference string for URN resolution
	ref := cp.Resolver.GetRef(nAttr, biblContent)

	// Get URN if ref is valid
	var urn string
	if ref != "" {
		urn = cp.Resolver.GetURN(ref, "", filename)
	}

	// Extract context around the citation
	context := cp.extractContext(biblContent, xmlContent, 200)

	return Citation{
		NAttrib:   nAttr,
		Bibl:      biblContent,
		Ref:       ref,
		URN:       urn,
		Quote:     quote,
		XMLContext: context,
		Filename:  filename,
		DocCitURN: citURN,
	}
}