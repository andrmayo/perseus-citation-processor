package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"perseus_citation_linker/pkg/resolver"
	"perseus_citation_linker/pkg/xmlparser"
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

	// Create JSON encoders with HTML escaping disabled for better readability
	resolvedEncoder := json.NewEncoder(resolvedFile)
	resolvedEncoder.SetEscapeHTML(false)

	unresolvedEncoder := json.NewEncoder(unresolvedFile)
	unresolvedEncoder.SetEscapeHTML(false)

	// Now, with file handling and errors out of the way, process all citations from channel
	for citation := range citations {
		if citation.URN != "" && citation.Ref != "" {
			// Successful resolution
			if err := resolvedEncoder.Encode(citation); err != nil {
				log.Printf("Error encoding citation: %v", err)
			}
		} else {
			if err := unresolvedEncoder.Encode(citation); err != nil {
				log.Printf("Error encoding citation: %v", err)
			}
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
	// Use the proper XML parser to extract citations
	parsedCitations, err := xmlparser.ExtractCitations(xmlContent)
	if err != nil {
		log.Printf("Error parsing XML from %s: %v", filename, err)
		return []Citation{}
	}

	var allCitations []Citation

	// Convert parsed citations to our Citation format and resolve URNs
	for _, parsedCit := range parsedCitations {
		cp.DocCitCountersMux.Lock()
		cp.DocCitCounters[docID]++
		citationNum := cp.DocCitCounters[docID]
		cp.DocCitCountersMux.Unlock()

		citURN := fmt.Sprintf(":citations-%d.%d", docID, citationNum)

		// Get reference string for URN resolution
		ref := cp.Resolver.GetRef(parsedCit.NAttribute, parsedCit.BiblText)

		// Resolve to URN
		var urn string
		if ref != "" {
			urn = cp.Resolver.GetURN(ref, parsedCit.Context, filename)
		}

		citation := Citation{
			NAttrib:    parsedCit.NAttribute,
			Bibl:       parsedCit.BiblText,
			Ref:        ref,
			URN:        urn,
			Quote:      parsedCit.QuoteText,
			XMLContext: parsedCit.Context,
			Filename:   filename,
			DocCitURN:  citURN,
		}

		allCitations = append(allCitations, citation)
	}

	return allCitations
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

	// Create JSON encoders with HTML escaping disabled for better readability
	resolvedEncoder := json.NewEncoder(resolvedFile)
	resolvedEncoder.SetEscapeHTML(false)

	unresolvedEncoder := json.NewEncoder(unresolvedFile)
	unresolvedEncoder.SetEscapeHTML(false)

	for _, citation := range citations {
		if citation.URN != "" && citation.Ref != "" {
			// Successfully resolved
			if err := resolvedEncoder.Encode(citation); err != nil {
				log.Printf("Error encoding citation: %v", err)
			}
		} else {
			// Failed to resolve
			if err := unresolvedEncoder.Encode(citation); err != nil {
				log.Printf("Error encoding citation: %v", err)
			}
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
