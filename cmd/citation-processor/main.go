package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	Parallel       int
}

type CitationProcessor struct {
	Config            Config
	Resolver          *resolver.URNResolver
	DocCounter        int
	DocCounterMux     sync.Mutex
	DocumentMapping   map[int]string
	DocCitCounters    map[int]int // Map from docID to citation counter for that document
	DocCitCountersMux sync.Mutex
}

// type for channel
type fileJob struct {
	filename string
	docID    int
}

func NewCitationProcessor(config Config) (*CitationProcessor, error) {
	urnResolver, err := resolver.NewURNResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	return &CitationProcessor{
		Config:          config,
		Resolver:        urnResolver,
		DocCounter:      0,
		DocumentMapping: make(map[int]string),
		DocCitCounters:  make(map[int]int),
	}, nil
}

func main() {
	// Parse command line flags
	noCitTags := flag.Bool("nocit", false, "Use <bibl> and <quote> tags to guide citation extraction (default: use <cit> tags)")
	inputDir := flag.String("input", ".", "Input directory containing XML files")
	outputDir := flag.String("output", "cit_data", "Output directory for JSONL files")
	parallel := flag.Int("parallel", 0, "Number of files to process concurrently, default to 0 for files = # threads, pass 1 for sequential processing")
	flag.Parse()

	config := Config{
		InputDir:       *inputDir,
		OutputDir:      *outputDir,
		ResolvedFile:   "resolved.jsonl",
		UnresolvedFile: "unresolved.jsonl",
		UseCitTags:     !*noCitTags,
		Parallel:       *parallel,
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

	if len(xmlFiles) == 0 {
		return fmt.Errorf("no XML files found in %s", cp.Config.InputDir)
	}

	maxWorkers := cp.Config.Parallel

	if maxWorkers < 0 {
		return fmt.Errorf("config error: parallel must be >= 0 but is %d", maxWorkers)
	}
	if maxWorkers == 0 {
		maxWorkers = runtime.NumCPU()
	}

	// maxWorkers must be <= number of files
	if maxWorkers > len(xmlFiles) {
		maxWorkers = len(xmlFiles)
	}

	fmt.Printf("Processing %d XML files with %d workers\n", len(xmlFiles), maxWorkers)

	if maxWorkers == 1 {
		// Sequential processing
		return cp.processSequential(xmlFiles)
	} else {
		return cp.processConcurrent(xmlFiles, maxWorkers)
	}
}

func (cp *CitationProcessor) processSequential(xmlFiles []string) error {
	for _, xmlFile := range xmlFiles {
		fmt.Printf("Processing %s...\n", xmlFile)
		if err := cp.ProcessXMLFile(xmlFile); err != nil {
			log.Printf("Error processing %s: %v", xmlFile, err)
			continue
		}
	}

	// Write document mappings to JSON file
	if err := cp.WriteDocumentMappings(); err != nil {
		return fmt.Errorf("failed to write document mappings: %w", err)
	}

	return nil
}

func (cp *CitationProcessor) processConcurrent(xmlFiles []string, maxWorkers int) error {
	jobs := make(chan fileJob, len(xmlFiles))
	citations := make(chan Citation, 1000) // the capacity here is somewhat arbitrary

	// To track worker completion
	var wg sync.WaitGroup

	// Start router goroutine to route citations to resolved/unresolved files
	writerDone := make(chan error, 1)
	go cp.routeCitations(citations, writerDone)

	// Start worker pool
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go cp.fileWorker(jobs, citations, &wg)
	}

	// Pre-assign document IDs for deterministic ordering
	// Ensures doc IDs are based on file order rather than processing order
	for i, xmlFile := range xmlFiles {
		jobs <- fileJob{
			filename: xmlFile,
			docID:    i + 1,
		}
	}
	close(jobs)

	// Wait for workers
	wg.Wait()

	close(citations)

	// Wait for writer to finish
	if err := <-writerDone; err != nil {
		return err
	}

	// Write document mappings to JSON file
	if err := cp.WriteDocumentMappings(); err != nil {
		return fmt.Errorf("failed to write document mappings: %w", err)
	}

	return nil
}

func (cp *CitationProcessor) fileWorker(jobs <-chan fileJob, citations chan<- Citation, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		fmt.Printf("Processing %s...\n", job.filename)

		// Set document ID
		cp.DocCounterMux.Lock()
		cp.DocCounter = job.docID
		cp.DocumentMapping[job.docID] = job.filename
		cp.DocCounterMux.Unlock()

		content, err := os.ReadFile(job.filename)
		if err != nil {
			log.Printf("Error reading %s: %v", job.filename, err)
			continue
		}

		extractedCitations := cp.ExtractCitations(string(content), job.filename, job.docID)

		// Send citations to writer channel
		for _, citation := range extractedCitations {
			citations <- citation
		}
	}
}

func (cp *CitationProcessor) routeCitations(citations <-chan Citation, done chan<- error) {
	resolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.ResolvedFile)
	unresolvedPath := filepath.Join(cp.Config.OutputDir, cp.Config.UnresolvedFile)

	resolvedFile, err := os.OpenFile(resolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		done <- err
		return
	}

	defer resolvedFile.Close()

	unresolvedFile, err := os.OpenFile(unresolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		done <- err
		return
	}

	defer unresolvedFile.Close()

	// Now, with file handling and errors out of the way, process all citations from channel
	for citation := range citations {
		jsonData, err := json.Marshal(citation)
		if err != nil {
			log.Printf("Error marshaling citation: %v", err)
			continue
		}

		if citation.URN != "" && citation.Ref != "" {
			// Successful resolution
			resolvedFile.Write(jsonData)
			resolvedFile.WriteString("\n")
		} else {
			unresolvedFile.Write(jsonData)
			unresolvedFile.WriteString("\n")
		}
	}

	done <- nil
}

// Note: this function is only used in sequential mode (i.e. with just one worker for XML files)
// Equivalent function for concurrent mode is fileWorker()
func (cp *CitationProcessor) ProcessXMLFile(filename string) error {
	// Increment document counter for new document
	cp.DocCounterMux.Lock()
	cp.DocCounter++
	currentDocID := cp.DocCounter
	cp.DocumentMapping[currentDocID] = filename
	cp.DocCounterMux.Unlock()

	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Extract citations from XML content
	citations := cp.ExtractCitations(string(content), filename, currentDocID)

	// Write citations to appropriate output files
	return cp.WriteCitations(citations)
}

func (cp *CitationProcessor) ExtractCitations(xmlContent, filename string, docID int) []Citation {
	// First, remove TEI header (sometimes contains metadata packaged in <bibl> tags)
	xmlContent = stripTEIHeader(xmlContent)

	var allCitations []Citation

	if cp.Config.UseCitTags {
		// Comprehensive extraction approach - find all citation patterns regardless of XML structure
		allCitations = cp.extractAllCitationPatterns(xmlContent, filename, docID)
	} else {
		// Original behavior: only extract <bibl> tags
		allCitations = cp.extractBiblTags(xmlContent, filename, docID)
	}

	return allCitations
}

func stripTEIHeader(xmlContent string) string {
	// Match <teiHeader> ... </teiHeader> (case-insensitive, dotall mode)
	headerRegex := regexp.MustCompile(`(?si)<teiHeader[^>]*>.*?</teiHeader>`)
	return headerRegex.ReplaceAllString(xmlContent, "")
}

// extractBiblTags extracts citations using <bibl> tags directly (original method)
func (cp *CitationProcessor) extractBiblTags(xmlContent, filename string, docID int) []Citation {
	// Regex to find <bibl> elements
	biblRegex := regexp.MustCompile(`(?s)<bibl[^>]*>.*?</bibl>`)
	matches := biblRegex.FindAllStringSubmatch(xmlContent, -1)

	var citations []Citation

	for _, match := range matches {
		if len(match) > 0 {
			// In -nocit mode, preserve original behavior: extract nearby quotes
			citation := cp.ProcessCitation(match[0], xmlContent, filename, true, docID)
			citations = append(citations, citation)
		}
	}

	return citations
}

// processCitationTag processes a single <cit> element containing <bibl> and <quote>
func (cp *CitationProcessor) processCitationTag(citMatch, xmlContent, filename string, docID int) Citation {
	cp.DocCitCountersMux.Lock()
	cp.DocCitCounters[docID]++
	citationNum := cp.DocCitCounters[docID]
	cp.DocCitCountersMux.Unlock()

	citURN := fmt.Sprintf(":citations-%d.%d", docID, citationNum)

	// Extract bibl element from within the cit tag
	biblRegex := regexp.MustCompile(`(?s)<bibl[^>]*>.*?</bibl>`)
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

func (cp *CitationProcessor) ProcessCitation(biblMatch, xmlContent, filename string, extractQuotes bool, docID int) Citation {
	cp.DocCitCountersMux.Lock()
	cp.DocCitCounters[docID]++
	citationNum := cp.DocCitCounters[docID]
	cp.DocCitCountersMux.Unlock()

	citURN := fmt.Sprintf(":citations-%d.%d", docID, citationNum)

	// Extract n attribute
	nAttr := cp.extractAttribute(biblMatch, "n")

	// Extract bibl content
	biblContent := cp.extractBiblContent(biblMatch)

	// Extract quote (look for quote element after bibl) - only if requested
	var quote string
	if extractQuotes {
		quote = cp.extractQuote(xmlContent, biblMatch)
	}

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
	re := regexp.MustCompile(`(?s)<bibl[^>]*>(.*?)</bibl>`)
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
	quoteRegex := regexp.MustCompile(`(?s)<quote[^>]*>(.*?)</quote>`)
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

func (cp *CitationProcessor) WriteDocumentMappings() error {
	mappingPath := filepath.Join(cp.Config.OutputDir, "document_mappings.json")

	// Create a sorted list of document IDs for consistent output
	type docMapping struct {
		ID       int    `json:"id"`
		Filename string `json:"filename"`
	}

	mappings := make([]docMapping, 0, len(cp.DocumentMapping))
	for id, filename := range cp.DocumentMapping {
		mappings = append(mappings, docMapping{ID: id, Filename: filename})
	}

	// Sort by ID to ensure consistent ordering
	for i := 0; i < len(mappings); i++ {
		for j := i + 1; j < len(mappings); j++ {
			if mappings[i].ID > mappings[j].ID {
				mappings[i], mappings[j] = mappings[j], mappings[i]
			}
		}
	}

	// Write to file with pretty formatting
	jsonData, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal document mappings: %w", err)
	}

	if err := os.WriteFile(mappingPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write document mappings file: %w", err)
	}

	fmt.Printf("Document mappings written to %s\n", mappingPath)
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
func (cp *CitationProcessor) extractAllCitationPatterns(xmlContent, filename string, docID int) []Citation {
	var allCitations []Citation

	// Pattern 1: Extract ALL <cit> elements anywhere in the document
	citRegex := regexp.MustCompile(`(?s)<cit\b[^>]*>.*?</cit>`)
	citMatches := citRegex.FindAllString(xmlContent, -1)

	for _, citMatch := range citMatches {
		citation := cp.processCitationTag(citMatch, xmlContent, filename, docID)
		if citation.Bibl != "" {
			allCitations = append(allCitations, citation)
		}
	}

	// Pattern 2: Extract ALL standalone <bibl> elements (not within <cit>)
	// First remove all <cit> containers to avoid double-counting
	contentWithoutCit := citRegex.ReplaceAllString(xmlContent, "")
	biblRegex := regexp.MustCompile(`(?s)<bibl\b[^>]*>.*?</bibl>`)
	biblMatches := biblRegex.FindAllString(contentWithoutCit, -1)

	for _, biblMatch := range biblMatches {
		// Don't extract quotes for standalone <bibl> tags in default mode
		citation := cp.ProcessCitation(biblMatch, xmlContent, filename, false, docID)
		if citation.Bibl != "" {
			allCitations = append(allCitations, citation)
		}
	}

	// Pattern 3: Look for <ref> elements that might contain citations
	// Be more selective - only include if they resolve to valid URNs
	refRegex := regexp.MustCompile(`<ref\b[^>]*>([^<]+)</ref>`)
	refMatches := refRegex.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range refMatches {
		if len(match) >= 2 {
			refContent := strings.TrimSpace(match[1])
			// Only consider ref content that looks like a real citation (has author.work pattern)
			if refContent != "" && regexp.MustCompile(`[A-Za-z]+\.\s*[A-Za-z]*\s*\d+`).MatchString(refContent) {
				citation := cp.createCitationFromParts("", refContent, "", xmlContent, filename, docID)
				if citation.Bibl != "" && citation.URN != "" {
					allCitations = append(allCitations, citation)
				}
			}
		}
	}

	return allCitations
}

// createCitationFromParts creates a Citation from individual components
func (cp *CitationProcessor) createCitationFromParts(nAttr, biblContent, quote, xmlContent, filename string, docID int) Citation {
	cp.DocCitCountersMux.Lock()
	cp.DocCitCounters[docID]++
	citationNum := cp.DocCitCounters[docID]
	cp.DocCitCountersMux.Unlock()

	citURN := fmt.Sprintf(":citations-%d.%d", docID, citationNum)

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
