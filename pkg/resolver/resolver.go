package resolver

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"perseus_citation_linker/pkg/loader"
)

type URNResolver struct {
	Data *loader.ComprehensiveData
}

func NewURNResolver() (*URNResolver, error) {
	data, err := loader.LoadComprehensiveData()
	if err != nil {
		return nil, fmt.Errorf("failed to load citation data: %w", err)
	}

	return &URNResolver{
		Data: data,
	}, nil
}

func (ur *URNResolver) GetRef(nAttr, biblContent string) string {
	// This implements the Python get_ref logic exactly
	if nAttr != "" {
		nAttr = strings.ToLower(strings.TrimSpace(nAttr))
	}
	if biblContent != "" {
		biblContent = strings.ToLower(strings.TrimSpace(biblContent))
	}

	// Clean both inputs
	refs := []string{nAttr, biblContent}
	for i, ref := range refs {
		if ref != "" {
			// Normalize all whitespace (including newlines, tabs) to single spaces
			ref = regexp.MustCompile(`\s+`).ReplaceAllString(ref, " ")
			ref = strings.TrimSpace(ref)

			// Remove HTML title tags
			ref = regexp.MustCompile(`<title.*?>`).ReplaceAllString(ref, "")
			ref = strings.ReplaceAll(ref, "</title>", "")
			// Remove parentheses
			ref = regexp.MustCompile(`[\(\)]`).ReplaceAllString(ref, "")
			// Replace ", " with " "
			ref = strings.ReplaceAll(ref, ", ", " ")
			// Deal with section symbols
			ref = regexp.MustCompile(` *§ *`).ReplaceAllString(ref, ".")
			// Deal with spacing issues with alphabetic page references
			ref = regexp.MustCompile(`(\d+) ([A-Za-z])`).ReplaceAllString(ref, "$1$2")
			refs[i] = ref
		}
	}
	nAttr, biblContent = refs[0], refs[1]

	// Early return conditions
	if biblContent == "" || strings.TrimSpace(biblContent) == "" {
		if nAttr != "" {
			return nAttr
		}
		return ""
	}
	if nAttr == "" || strings.TrimSpace(nAttr) == "" {
		return biblContent
	}

	// Check if n attribute contains URN
	if ur.detectURN(nAttr) != "" {
		return nAttr
	}

	// Pattern matching logic - best to worst patterns
	patterns := []string{
		// Best: author work number.number
		`([a-zA-Z]+\.?\s?[a-zA-Z]*) ([a-zA-Z]+\.?\s?[a-zA-Z]*) \d+(\s|\.|:)\d+`,
		// Second best: author work number
		`([a-zA-Z]+\.?\s?[a-zA-Z]*) ([a-zA-Z]+\.?\s?[a-zA-Z]*) \d+`,
		// Third best: author number.number
		`([a-zA-Z]+\.?) \d+(\s|\.|:)\d+`,
		// Fourth best: author number
		`([a-zA-Z]+\.?) \d+`,
	}

	allAuthAbb := ur.Data.GetAllAuthAbb()
	allAuthors := ur.Data.GetAllAuthors()

	for _, pattern := range patterns {
		// Try n attribute first
		if matched, _ := regexp.MatchString(pattern, nAttr); matched {
			split := strings.Fields(nAttr)
			if ur.hasRecognizedAuthor(split, allAuthAbb, allAuthors) {
				return nAttr
			}
		}

		// Try bibl content
		if matched, _ := regexp.MatchString(pattern, biblContent); matched {
			split := strings.Fields(biblContent)
			if ur.hasRecognizedAuthor(split, allAuthAbb, allAuthors) {
				return biblContent
			}
		}
	}

	// Check for recognized authors without pattern matching
	nAuthRec := ur.hasRecognizedAuthor(strings.Fields(nAttr), allAuthAbb, allAuthors)
	biblAuthRec := ur.hasRecognizedAuthor(strings.Fields(biblContent), allAuthAbb, allAuthors)

	if nAuthRec && !biblAuthRec {
		return nAttr
	}
	if biblAuthRec && !nAuthRec {
		return biblContent
	}

	// Both have recognized authors - check for recognized works
	if nAuthRec && biblAuthRec {
		if ur.hasRecognizedWork(nAttr, allAuthAbb, allAuthors) {
			return nAttr
		}
		if ur.hasRecognizedWork(biblContent, allAuthAbb, allAuthors) {
			return biblContent
		}
	}

	return ""
}

func (ur *URNResolver) hasRecognizedAuthor(split []string, authAbb map[string]interface{}, authors map[string]bool) bool {
	if len(split) == 0 {
		return false
	}

	// Check unigram, bigram, trigram
	for i := 1; i <= 3 && i <= len(split); i++ {
		candidate := strings.Join(split[:i], " ")
		if _, exists := authAbb[candidate]; exists {
			return true
		}
		if authors[candidate] {
			return true
		}
	}
	return false
}

func (ur *URNResolver) hasRecognizedWork(ref string, authAbb map[string]interface{}, authors map[string]bool) bool {
	split := strings.Fields(ref)
	if len(split) < 2 {
		return false
	}

	// Find author
	var author string
	var authLen int
	for i := 1; i <= 3 && i <= len(split); i++ {
		candidate := strings.Join(split[:i], " ")
		if val, exists := authAbb[candidate]; exists {
			if str, ok := val.(string); ok {
				author = str
				authLen = i
				break
			}
		}
		if authors[candidate] {
			author = candidate
			authLen = i
			break
		}
	}

	if author == "" || authLen >= len(split) {
		return false
	}

	// Check for recognized work
	allWorkURNs := ur.Data.GetAllWorkURNs()
	authorWorks, exists := allWorkURNs[author]
	if !exists {
		return false
	}

	workPart := strings.Join(split[authLen:], " ")
	// Remove any numeric parts
	workPart = regexp.MustCompile(`\d.*`).ReplaceAllString(workPart, "")
	workPart = strings.TrimSpace(workPart)

	// Check for work up to trigram
	workSplit := strings.Fields(workPart)
	for i := 1; i <= 3 && i <= len(workSplit); i++ {
		candidate := strings.Join(workSplit[:i], " ")
		if _, exists := authorWorks[candidate]; exists {
			return true
		}
	}

	return false
}

func (ur *URNResolver) GetURN(ref, context, filename string) string {
	if ref == "" {
		return ""
	}

	// Handle "ff" notation
	if strings.HasSuffix(ref, "ff") {
		if len(ref) > 2 && ref[len(ref)-3] == ' ' {
			ref = ref[:len(ref)-3] + ref[len(ref)-2:]
		}
	} else if strings.HasSuffix(ref, "ff.") {
		if len(ref) > 3 && ref[len(ref)-4] == ' ' {
			ref = ref[:len(ref)-4] + "ff"
		} else {
			ref = ref[:len(ref)-3] + "ff"
		}
	}

	// Detect if ref is already a URN
	if urnPart := ur.detectURN(ref); urnPart != "" {
		return ur.formatExistingURN(ref, urnPart)
	}

	// Parse reference
	author, work, passage := ur.parseReference(ref)
	if author == "" {
		log.Printf("No author found in reference: %s", ref)
		return ""
	}

	// Resolve author abbreviation
	resolvedAuthor := ur.resolveAuthor(author, work)
	if resolvedAuthor == "" {
		log.Printf("Author not recognized: %s", author)
		return ""
	}

	// Handle single work authors
	if ur.Data.IsSingleWorkAuthor(resolvedAuthor) {
		// For single work authors, treat work field as part of passage if it looks like a book/section reference
		if work != "" && ur.looksLikeBookReference(work) {
			// Combine work and passage as location
			fullPassage := work
			if passage != "" {
				fullPassage += "." + passage
			}
			return ur.handleSingleWorkAuthor(resolvedAuthor, fullPassage, ref)
		} else if work == "" {
			return ur.handleSingleWorkAuthor(resolvedAuthor, passage, ref)
		}
	}

	// Get author URN
	allAuthURNs := ur.Data.GetAllAuthURNs()
	authURN, exists := allAuthURNs[resolvedAuthor]
	if !exists {
		log.Printf("No URN found for author: %s", resolvedAuthor)
		return ""
	}

	// Get work URN
	workURN := ur.getWorkURN(resolvedAuthor, work)
	if workURN == "" {
		log.Printf("No work URN found for %s: %s", resolvedAuthor, work)
		return ""
	}

	// Determine literature type for suffix
	suffix := ur.determineLiteratureSuffix(authURN)

	// Construct final URN
	if passage != "" {
		return fmt.Sprintf("%s.%s.%s:%s", authURN, workURN, suffix, passage)
	}
	return fmt.Sprintf("%s.%s.%s", authURN, workURN, suffix)
}

func (ur *URNResolver) detectURN(ref string) string {
	patterns := []string{
		`tlg\d+\.tlg\d+(:\d+.?\d*)?(ff)?`,
		`phi\d+\.phi\d+(:\d+.?\d*)?(ff)?`,
		`stoa\d+\.stoa\d+(:\d+.?\d*)?(ff)?`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(ref); match != "" {
			return match
		}
	}
	return ""
}

func (ur *URNResolver) formatExistingURN(ref, urnPart string) string {
	// Extract location after URN
	index := strings.Index(ref, urnPart)
	remaining := ref[index+len(urnPart):]

	locMatch := regexp.MustCompile(`\d+.*`)
	loc := ""
	if match := locMatch.FindString(remaining); match != "" {
		loc = match
	}

	// Determine URN type and format
	if strings.Contains(urnPart, "tlg") {
		base := urnPart
		if !strings.Contains(base, "urn:cts:greekLit") {
			base = "urn:cts:greekLit:" + base
		}
		if loc != "" {
			return fmt.Sprintf("%s.perseus-grc2:%s", base, loc)
		}
		return base + ".perseus-grc2"
	} else if strings.Contains(urnPart, "phi") {
		base := urnPart
		if !strings.Contains(base, "urn:cts:latinLit") {
			base = "urn:cts:latinLit:" + base
		}
		if loc != "" {
			return fmt.Sprintf("%s.perseus-lat2:%s", base, loc)
		}
		return base + ".perseus-lat2"
	}

	return urnPart
}

func (ur *URNResolver) parseReference(ref string) (author, work, passage string) {
	// Follow Python get_urn parsing logic more closely
	ref = strings.TrimSpace(ref)
	split := strings.Fields(ref)

	if len(split) == 0 {
		return "", "", ""
	}

	// Try to identify author (can be unigram or bigram)
	allAuthAbb := ur.Data.GetAllAuthAbb()
	allAuthors := ur.Data.GetAllAuthors()

	author = split[0]
	authLen := 1

	// Check for bigram author
	if len(split) > 1 {
		bigram := strings.Join(split[:2], " ")
		if _, exists := allAuthAbb[bigram]; exists {
			author = bigram
			authLen = 2
		} else if allAuthors[bigram] {
			author = bigram
			authLen = 2
		}
	}

	if authLen >= len(split) {
		return author, "", ""
	}

	// Process remaining parts for work and passage
	remaining := strings.Join(split[authLen:], " ")

	// Replace spaces in multi-word titles with underscores
	remaining = ur.processWorkTitles(author, remaining)

	// Handle various formats
	parts := strings.Fields(remaining)
	if len(parts) == 0 {
		return author, "", ""
	}

	// Check for work.passage format
	if len(parts) == 1 && strings.Contains(parts[0], ".") {
		workPassage := strings.SplitN(parts[0], ".", 2)
		if len(workPassage) == 2 {
			return author, workPassage[0], workPassage[1]
		}
	}

	// Find where passage starts (first numeric or Roman numeral part)
	workParts := []string{}
	for i, part := range parts {
		// Check if this part starts with a number or is a Roman numeral
		if regexp.MustCompile(`^\d`).MatchString(part) || ur.looksLikeRomanNumeral(part) {
			// This part starts with a number or is a Roman numeral - it's the passage
			work = strings.Join(workParts, " ")
			passage = strings.Join(parts[i:], " ")
			// Clean up passage formatting
			passage = regexp.MustCompile(`\s+`).ReplaceAllString(passage, ".")
			passage = strings.Trim(passage, ".")
			// Remove duplicate dots
			passage = regexp.MustCompile(`\.+`).ReplaceAllString(passage, ".")
			return author, work, passage
		}
		workParts = append(workParts, part)
	}

	// No numeric part found - everything is work
	work = strings.Join(parts, " ")
	return author, work, ""
}

func (ur *URNResolver) processWorkTitles(author, remaining string) string {
	// Replace spaces with underscores for multi-word titles
	allWorkURNs := ur.Data.GetAllWorkURNs()

	resolvedAuthor := ur.resolveAuthor(author, "")
	if resolvedAuthor == "" {
		return remaining
	}

	authorWorks, exists := allWorkURNs[resolvedAuthor]
	if !exists {
		return remaining
	}

	words := strings.Fields(remaining)
	if len(words) < 2 {
		return remaining
	}

	// Try progressively longer work titles
	for i := 2; i <= len(words); i++ {
		candidate := strings.Join(words[:i], " ")
		if _, exists := authorWorks[strings.ToLower(candidate)]; exists {
			// Found a match - replace spaces with underscores
			underscored := strings.Join(words[:i], "_")
			rest := strings.Join(words[i:], " ")
			if rest != "" {
				return underscored + " " + rest
			}
			return underscored
		}
	}

	return remaining
}

func (ur *URNResolver) resolveAuthor(author, work string) string {
	allAuthAbb := ur.Data.GetAllAuthAbb()
	allAuthors := ur.Data.GetAllAuthors()

	author = strings.ToLower(author)

	// Check direct match first
	if allAuthors[author] {
		return author
	}

	// Check abbreviations
	if val, exists := allAuthAbb[author]; exists {
		if str, ok := val.(string); ok {
			// If it's a function reference (like "_which_pliny"), use the resolver
			if str == "_which_pliny" || str == "_which_seneca" {
				return ur.Data.ResolveLatinAuthorFunction(author, work)
			}
			return str
		}
		// Handle function cases (Pliny/Seneca disambiguation)
		return ur.Data.ResolveLatinAuthorFunction(author, work)
	}

	return ""
}

func (ur *URNResolver) handleSingleWorkAuthor(author, passage, originalRef string) string {
	allAuthURNs := ur.Data.GetAllAuthURNs()
	authURN, exists := allAuthURNs[author]
	if !exists {
		return ""
	}

	// Default work is tlg001
	workURN := "tlg001"
	suffix := ur.determineLiteratureSuffix(authURN)

	// Extract numeric parts for location
	numerics := []string{}
	parts := regexp.MustCompile(`[\s,.:]`).Split(originalRef, -1)
	for _, part := range parts {
		if regexp.MustCompile(`^\d+`).MatchString(part) {
			numerics = append(numerics, part)
		}
	}

	if len(numerics) > 0 {
		loc := strings.Join(numerics, ".")
		return fmt.Sprintf("%s.%s.%s:%s", authURN, workURN, suffix, loc)
	}

	return fmt.Sprintf("%s.%s.%s", authURN, workURN, suffix)
}

func (ur *URNResolver) getWorkURN(author, work string) string {
	allWorkURNs := ur.Data.GetAllWorkURNs()
	authorWorks, exists := allWorkURNs[author]
	if !exists {
		// No work mappings for this author - apply fallback strategies
		work = strings.ToLower(work)

		// Handle numeric work IDs (from Python logic lines 760-771)
		if ur.isNumeric(work) {
			return ur.constructNumericWorkURN(author, work)
		}

		// Final fallback: use primary work tlg001 (from Python logic lines 780-787)
		return "tlg001"
	}

	work = strings.ToLower(work)

	// First priority: exact match
	if urn, exists := authorWorks[work]; exists {
		if str, ok := urn.(string); ok {
			return str
		}
		// Handle tuple cases (like Demosthenes orations)
		if slice, ok := urn.([]interface{}); ok && len(slice) >= 3 {
			return ur.handleWorkRange(work, slice)
		}
	}

	// Second priority: try abbreviations but prefer exact matches over generated ones
	var exactMatches []string
	var abbreviationMatches []string

	for title := range authorWorks {
		// Check if this title exactly matches the work
		if title == work {
			if urn, exists := authorWorks[title]; exists {
				if str, ok := urn.(string); ok {
					exactMatches = append(exactMatches, str)
				}
			}
		} else {
			// Check generated abbreviations
			abbrevs := loader.GenerateWorkAbbreviations(title)
			for _, abbrev := range abbrevs {
				if abbrev == work {
					if urn, exists := authorWorks[title]; exists {
						if str, ok := urn.(string); ok {
							abbreviationMatches = append(abbreviationMatches, str)
						}
					}
					break // Only add once per title
				}
			}
		}
	}

	// Return first exact match if any
	if len(exactMatches) > 0 {
		return exactMatches[0]
	}

	// Return first abbreviation match if any
	if len(abbreviationMatches) > 0 {
		return abbreviationMatches[0]
	}

	// Handle numeric work IDs (from Python logic lines 760-771)
	if ur.isNumeric(work) {
		return ur.constructNumericWorkURN(author, work)
	}

	// Final fallback: use primary work tlg001 (from Python logic lines 780-787)
	// This handles cases where work is assumed to be author's main work
	return "tlg001"
}

// isNumeric checks if a string contains only digits
func (ur *URNResolver) isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// constructNumericWorkURN creates URN for numeric work IDs like "19" -> "tlg019"
func (ur *URNResolver) constructNumericWorkURN(author, work string) string {
	allAuthURNs := ur.Data.GetAllAuthURNs()
	authURN, exists := allAuthURNs[author]
	if !exists {
		return ""
	}

	var prefix string
	if strings.Contains(authURN, "greekLit") {
		prefix = "tlg"
	} else if strings.Contains(authURN, "latinLit") {
		prefix = "phi"
	} else {
		return ""
	}

	// Python logic: if work starts with "0", use as-is, otherwise prepend "0"
	if strings.HasPrefix(work, "0") {
		return prefix + work
	} else {
		return prefix + "0" + work
	}
}

func (ur *URNResolver) handleWorkRange(work string, tuple []interface{}) string {
	if len(tuple) < 3 {
		return ""
	}

	prefix, ok1 := tuple[0].(string)
	startFloat, ok2 := tuple[1].(float64)
	endFloat, ok3 := tuple[2].(float64)

	if !ok1 || !ok2 || !ok3 {
		return ""
	}

	start := int(startFloat)
	end := int(endFloat)

	// Extract number from work
	re := regexp.MustCompile(`\d+`)
	matches := re.FindStringSubmatch(work)
	if len(matches) > 0 {
		if num, err := strconv.Atoi(matches[0]); err == nil {
			if num >= start && num <= end {
				return fmt.Sprintf("%s%03d", prefix, num)
			}
		}
	}

	return ""
}

func (ur *URNResolver) looksLikeBookReference(work string) bool {
	work = strings.ToLower(strings.TrimSpace(work))

	// Roman numerals (common for book references in ancient texts)
	romanNumerals := []string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x",
							 "xi", "xii", "xiii", "xiv", "xv", "xvi", "xvii", "xviii", "xix", "xx"}

	for _, roman := range romanNumerals {
		if work == roman || work == roman+"." {
			return true
		}
	}

	// Arabic numerals
	if matched, _ := regexp.MatchString(`^\d+\.?$`, work); matched {
		return true
	}

	return false
}

func (ur *URNResolver) looksLikeRomanNumeral(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimSuffix(text, ".")

	// Roman numerals (common for book references in ancient texts)
	romanNumerals := []string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x",
							 "xi", "xii", "xiii", "xiv", "xv", "xvi", "xvii", "xviii", "xix", "xx"}
	for _, roman := range romanNumerals {
		if text == roman {
			return true
		}
	}
	return false
}

func (ur *URNResolver) determineLiteratureSuffix(authURN string) string {
	if strings.Contains(authURN, "greekLit") {
		return "perseus-grc2"
	} else if strings.Contains(authURN, "latinLit") {
		return "perseus-lat2"
	} else if strings.Contains(authURN, "englishLit") {
		return "perseus-eng2"
	} else if strings.Contains(authURN, "greekSchol") {
		return "perseus-grc2"
	}
	return "perseus-grc2" // default
}
