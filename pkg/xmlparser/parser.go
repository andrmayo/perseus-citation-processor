package xmlparser

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// Citation represents a citation extracted from XML
type Citation struct {
	NAttribute string
	BiblText   string
	QuoteText  string
	Context    string
}

// contextTracker maintains a sliding window of recent XML tokens for context
type contextTracker struct {
	buffer    []string
	totalSize int
	maxSize   int
}

func newContextTracker(maxSize int) *contextTracker {
	return &contextTracker{
		buffer:  make([]string, 0, 100),
		maxSize: maxSize,
	}
}

func (ct *contextTracker) add(s string) {
	ct.buffer = append(ct.buffer, s)
	ct.totalSize += len(s)

	// Trim from the front if we exceed maxSize
	for ct.totalSize > ct.maxSize && len(ct.buffer) > 0 {
		ct.totalSize -= len(ct.buffer[0])
		ct.buffer = ct.buffer[1:]
	}
}

func (ct *contextTracker) getContext() string {
	return strings.Join(ct.buffer, "")
}

// preprocessXML removes custom entity references that would cause the XML parser to fail.
// Standard XML entities (&lt;, &gt;, &amp;, &quot;, &apos;) are preserved.
func preprocessXML(xmlContent string) string {
	// Remove custom entity references like &responsibility;, &fund.Tufts;, etc.
	// but preserve standard XML entities
	standardEntities := map[string]bool{
		"lt":   true,
		"gt":   true,
		"amp":  true,
		"quot": true,
		"apos": true,
	}

	// Match entity references like &foo; or &foo.bar;
	re := regexp.MustCompile(`&([a-zA-Z][a-zA-Z0-9._-]*);`)

	result := re.ReplaceAllStringFunc(xmlContent, func(match string) string {
		// Extract entity name from &name;
		entityName := match[1 : len(match)-1]

		// Keep standard XML entities
		if standardEntities[entityName] {
			return match
		}

		// Remove custom entities (replace with empty string)
		return ""
	})

	return result
}

// ExtractCitations extracts all citations from TEI XML content
// It properly handles <cit> elements with nested <bibl> and <quote>,
// and standalone <bibl> elements outside of <cit> tags
func ExtractCitations(xmlContent string) ([]Citation, error) {
	// Preprocess XML to remove custom entity references
	xmlContent = preprocessXML(xmlContent)

	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	var citations []Citation
	var inTEIHeader bool
	var inText bool
	var lastBiblIndex int = -1 // Track index of last standalone bibl to potentially add quote

	// Track context with sliding windows
	beforeContext := newContextTracker(500)
	afterContext := newContextTracker(500)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			// Track whether we're in teiHeader (skip it) or text (process it)
			if elem.Name.Local == "teiHeader" {
				inTEIHeader = true
			} else if elem.Name.Local == "text" {
				inText = true
			}

			// Only process citations if we're in <text> and not in <teiHeader>
			if !inTEIHeader && inText {
				if elem.Name.Local == "cit" {
					// Capture context before processing
					contextBefore := beforeContext.getContext()

					citation, xmlRep, err := extractCitElement(decoder, elem)
					if err != nil {
						return nil, err
					}
					if citation != nil {
						// Build context: before + citation (after will be added as we continue parsing)
						citation.Context = contextBefore + xmlRep
						citations = append(citations, *citation)

						// Add citation to both trackers
						beforeContext.add(xmlRep)
						afterContext.add(xmlRep)
					}
					lastBiblIndex = -1
				} else if elem.Name.Local == "bibl" {
					// Capture context before processing
					contextBefore := beforeContext.getContext()

					citation, xmlRep, err := extractBiblElement(decoder, elem)
					if err != nil {
						return nil, err
					}
					if citation != nil {
						citation.Context = contextBefore + xmlRep
						citations = append(citations, *citation)
						lastBiblIndex = len(citations) - 1

						// Add to trackers
						beforeContext.add(xmlRep)
						afterContext.add(xmlRep)
					}
				} else if elem.Name.Local == "quote" && lastBiblIndex >= 0 {
					// This quote might be associated with the last standalone bibl
					quoteText, quoteXML, err := extractQuoteElement(decoder, elem)
					if err != nil {
						return nil, err
					}
					// Update the last bibl citation with quote
					citations[lastBiblIndex].QuoteText = quoteText
					citations[lastBiblIndex].Context += " " + quoteXML

					// Add to trackers
					beforeContext.add(" " + quoteXML)
					afterContext.add(" " + quoteXML)
					lastBiblIndex = -1
				} else {
					// Add other start elements to context trackers
					var elemStr strings.Builder
					elemStr.WriteString("<")
					elemStr.WriteString(elem.Name.Local)
					for _, attr := range elem.Attr {
						elemStr.WriteString(" ")
						elemStr.WriteString(attr.Name.Local)
						elemStr.WriteString("=\"")
						elemStr.WriteString(attr.Value)
						elemStr.WriteString("\"")
					}
					elemStr.WriteString(">")

					elemString := elemStr.String()
					beforeContext.add(elemString)
					afterContext.add(elemString)

					// Add to existing citation contexts if we're building after-context
					if len(citations) > 0 && afterContext.totalSize < afterContext.maxSize {
						// Add to recent citations' contexts
						startIdx := len(citations) - 1
						for i := startIdx; i >= 0 && i >= startIdx-10; i-- {
							if len(citations[i].Context) < 1000 { // Don't let contexts grow unbounded
								citations[i].Context += elemString
							}
						}
					}

					lastBiblIndex = -1
				}
			}

		case xml.EndElement:
			if elem.Name.Local == "teiHeader" {
				inTEIHeader = false
			} else if elem.Name.Local == "text" {
				inText = false
			}

			// Add end elements to context trackers
			if !inTEIHeader && inText {
				endTag := "</" + elem.Name.Local + ">"
				beforeContext.add(endTag)
				afterContext.add(endTag)

				// Add to recent citations' after-contexts
				if len(citations) > 0 && afterContext.totalSize < afterContext.maxSize {
					startIdx := len(citations) - 1
					for i := startIdx; i >= 0 && i >= startIdx-10; i-- {
						if len(citations[i].Context) < 1000 {
							citations[i].Context += endTag
						}
					}
				}
			}

		case xml.CharData:
			// Add character data to context trackers
			if !inTEIHeader && inText {
				charData := string(elem)
				beforeContext.add(charData)
				afterContext.add(charData)

				// Add to recent citations' after-contexts
				if len(citations) > 0 && afterContext.totalSize < afterContext.maxSize {
					startIdx := len(citations) - 1
					for i := startIdx; i >= 0 && i >= startIdx-10; i-- {
						if len(citations[i].Context) < 1000 {
							citations[i].Context += charData
						}
					}
				}

				// Check if it's non-whitespace content between bibl and quote
				if lastBiblIndex >= 0 && len(strings.TrimSpace(charData)) > 0 {
					lastBiblIndex = -1
				}
			}
		}
	}

	return citations, nil
}

// extractCitElement extracts a citation from a <cit> element
// Expected structure: <cit><bibl>...</bibl><quote>...</quote></cit>
// Returns: citation, XML representation, error
func extractCitElement(decoder *xml.Decoder, citStart xml.StartElement) (*Citation, string, error) {
	citation := &Citation{}
	depth := 1 // Track nesting depth
	var xmlBuilder strings.Builder

	// Reconstruct opening tag
	xmlBuilder.WriteString("<")
	xmlBuilder.WriteString(citStart.Name.Local)
	for _, attr := range citStart.Attr {
		xmlBuilder.WriteString(" ")
		xmlBuilder.WriteString(attr.Name.Local)
		xmlBuilder.WriteString("=\"")
		xmlBuilder.WriteString(attr.Value)
		xmlBuilder.WriteString("\"")
	}
	xmlBuilder.WriteString(">")

	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, "", err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			depth++
			if elem.Name.Local == "bibl" {
				// Extract n attribute
				for _, attr := range elem.Attr {
					if attr.Name.Local == "n" {
						citation.NAttribute = attr.Value
						break
					}
				}

				// Get bibl text content
				var biblXML strings.Builder
				biblXML.WriteString("<")
				biblXML.WriteString(elem.Name.Local)
				for _, attr := range elem.Attr {
					biblXML.WriteString(" ")
					biblXML.WriteString(attr.Name.Local)
					biblXML.WriteString("=\"")
					biblXML.WriteString(attr.Value)
					biblXML.WriteString("\"")
				}
				biblXML.WriteString(">")

				textContent, err := extractTextContent(decoder, &biblXML)
				if err != nil {
					return nil, "", err
				}
				citation.BiblText = textContent

				xmlBuilder.WriteString(biblXML.String())
				depth-- // extractTextContent consumed the end tag
			} else if elem.Name.Local == "quote" {
				// Get quote text content
				var quoteXML strings.Builder
				quoteXML.WriteString("<")
				quoteXML.WriteString(elem.Name.Local)
				for _, attr := range elem.Attr {
					quoteXML.WriteString(" ")
					quoteXML.WriteString(attr.Name.Local)
					quoteXML.WriteString("=\"")
					quoteXML.WriteString(attr.Value)
					quoteXML.WriteString("\"")
				}
				quoteXML.WriteString(">")

				textContent, err := extractTextContent(decoder, &quoteXML)
				if err != nil {
					return nil, "", err
				}
				citation.QuoteText = textContent

				xmlBuilder.WriteString(quoteXML.String())
				depth-- // extractTextContent consumed the end tag
			} else {
				// Write other start elements to XML
				xmlBuilder.WriteString("<")
				xmlBuilder.WriteString(elem.Name.Local)
				for _, attr := range elem.Attr {
					xmlBuilder.WriteString(" ")
					xmlBuilder.WriteString(attr.Name.Local)
					xmlBuilder.WriteString("=\"")
					xmlBuilder.WriteString(attr.Value)
					xmlBuilder.WriteString("\"")
				}
				xmlBuilder.WriteString(">")
			}

		case xml.EndElement:
			depth--
			if depth == 0 {
				xmlBuilder.WriteString("</")
				xmlBuilder.WriteString(elem.Name.Local)
				xmlBuilder.WriteString(">")
				return citation, xmlBuilder.String(), nil
			}
			xmlBuilder.WriteString("</")
			xmlBuilder.WriteString(elem.Name.Local)
			xmlBuilder.WriteString(">")

		case xml.CharData:
			xmlBuilder.Write(elem)
		}
	}
}

// extractBiblElement extracts a citation from a standalone <bibl> element
// These are <bibl> tags that are NOT nested within a <cit> element
// Returns: citation, XML representation, error
func extractBiblElement(decoder *xml.Decoder, biblStart xml.StartElement) (*Citation, string, error) {
	citation := &Citation{
		QuoteText: "", // Standalone bibl has empty quote (may be filled in later)
	}

	var xmlBuilder strings.Builder

	// Reconstruct opening tag
	xmlBuilder.WriteString("<")
	xmlBuilder.WriteString(biblStart.Name.Local)
	for _, attr := range biblStart.Attr {
		if attr.Name.Local == "n" {
			citation.NAttribute = attr.Value
		}
		xmlBuilder.WriteString(" ")
		xmlBuilder.WriteString(attr.Name.Local)
		xmlBuilder.WriteString("=\"")
		xmlBuilder.WriteString(attr.Value)
		xmlBuilder.WriteString("\"")
	}
	xmlBuilder.WriteString(">")

	// Extract text content
	textContent, err := extractTextContent(decoder, &xmlBuilder)
	if err != nil {
		return nil, "", err
	}
	citation.BiblText = textContent

	return citation, xmlBuilder.String(), nil
}

// extractQuoteElement extracts text from a <quote> element
// Returns: quote text, XML representation, error
func extractQuoteElement(decoder *xml.Decoder, quoteStart xml.StartElement) (string, string, error) {
	var xmlBuilder strings.Builder

	// Reconstruct opening tag
	xmlBuilder.WriteString("<")
	xmlBuilder.WriteString(quoteStart.Name.Local)
	for _, attr := range quoteStart.Attr {
		xmlBuilder.WriteString(" ")
		xmlBuilder.WriteString(attr.Name.Local)
		xmlBuilder.WriteString("=\"")
		xmlBuilder.WriteString(attr.Value)
		xmlBuilder.WriteString("\"")
	}
	xmlBuilder.WriteString(">")

	// Extract text content
	textContent, err := extractTextContent(decoder, &xmlBuilder)
	if err != nil {
		return "", "", err
	}

	return textContent, xmlBuilder.String(), nil
}

// extractTextContent extracts all text content until the matching end element
// Also appends the XML representation to the xmlBuilder
func extractTextContent(decoder *xml.Decoder, xmlBuilder *strings.Builder) (string, error) {
	var textBuilder strings.Builder
	depth := 1

	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			depth++
			xmlBuilder.WriteString("<")
			xmlBuilder.WriteString(elem.Name.Local)
			for _, attr := range elem.Attr {
				xmlBuilder.WriteString(" ")
				xmlBuilder.WriteString(attr.Name.Local)
				xmlBuilder.WriteString("=\"")
				xmlBuilder.WriteString(attr.Value)
				xmlBuilder.WriteString("\"")
			}
			xmlBuilder.WriteString(">")

		case xml.EndElement:
			depth--
			xmlBuilder.WriteString("</")
			xmlBuilder.WriteString(elem.Name.Local)
			xmlBuilder.WriteString(">")
			if depth == 0 {
				return strings.TrimSpace(textBuilder.String()), nil
			}

		case xml.CharData:
			text := string(elem)
			textBuilder.WriteString(text)
			xmlBuilder.WriteString(text)
		}
	}
}
