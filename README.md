# Perseus Citation Linker

A comprehensive Go implementation of the Perseus citation processing pipeline
for extracting and resolving citations from XML documents to CTS URNs
(Canonical Text Services Uniform Resource Names).
This was mainly developed for working with commentaries on ancient works.

An earlier Python version of the citation resolution logic is in place
as part of the data processing pipeline (<https://github.com/scaife-viewer/atlas-data-prep>)
for the Atlas backend for the Perseus Digital Library.
Part of the motivation here is to make it straightforward to collect citation instances
(mostly) correct resolutions for citations
that are not straightforwardly machine-actionable,
with an eye to automating citation tagging in XML documents at some point in the future.

## Features

- **Flexible Citation Extraction**: Supports two extraction modes:
  - Default mode for citations in preferred TEI Epidoc format:
    Extraction from any XML structure containing citation patterns contained within `<cit>` (the preferred TEI Epidoc format)
  - No cit tag mode: Extraction where `<bibl>` and `<quote>` are used without being bested with `<cit>` tags
- **Advanced Reference Resolution**: Resolves author/work abbreviations with dynamic disambiguation (e.g., Pliny Senior vs Junior)
- **Work Abbreviation Generation**: Automatically generates multiple abbreviation variants for work titles
- **CTS URN Generation**: Creates standardized URNs following Canonical Text Services format
- **Coverage**: Achieves 100% citation resolution on test datasets (both Sophoclean), but is currently better with commentaries on Greek works over Latin works
- **Concurrent Processing**: Efficiently handles multiple XML files
- **Comprehensive Testing**: Full test suite with expected vs actual output validation

## Installation & Usage

### Building

```bash
go build -o citation-processor cmd/citation-processor/main.go
```

### Running

```bash
# Process XML files with comprehensive <cit> tag extraction (recommended)
go run cmd/citation-processor/main.go -input testdata/xml/ -cit

# Process with traditional <bibl> tag extraction only
go run cmd/citation-processor/main.go -input testdata/xml/

# Specify custom output directory
go run cmd/citation-processor/main.go -input testdata/xml/ -cit -output results/
```

### Command Line Options

- `-input <directory>`: Directory containing XML files to process (default: current directory)
- `-output <directory>`: Output directory for results (default: "cit_data")
- `-cit`: Use comprehensive <cit> tag extraction mode (default: <bibl> tag only)

## Data Directory Configuration

### Automatic Data Directory Discovery

The application automatically searches for the `data/` directory containing citation mapping files (JSON files with author/work mappings). The search follows this order:

1. Current working directory (`./data`)
2. Parent directory (`../data`)
3. Up to 3 levels up (`../../data`, `../../../data`)
4. Falls back to `data` if not found (will fail if data files are missing)

This allows the application to work correctly whether you run it from:
- The project root: `go run cmd/citation-processor/main.go`
- The cmd directory: `go run ./citation-processor/main.go`
- After installing the binary elsewhere (as long as data is accessible)

### Using as a Library with Custom Data Directories

If you're importing this package into your own Go project, you can specify a custom data directory:

```go
package main

import (
    "log"
    "github.com/yourname/perseus-citation-processor/pkg/loader"
    "github.com/yourname/perseus-citation-processor/pkg/resolver"
)

func main() {
    // Option 1: Load data from custom directory
    data, err := loader.LoadComprehensiveDataDir("/opt/myapp/citation-data")
    if err != nil {
        log.Fatal(err)
    }

    // Create resolver with loaded data
    resolver := &resolver.URNResolver{Data: data}
    urn := resolver.GetURN("soph. ot 151", "", "")

    // Option 2: Use convenience constructor
    resolver2, err := resolver.NewURNResolverFromDir("/opt/myapp/citation-data")
    if err != nil {
        log.Fatal(err)
    }
    urn2 := resolver2.GetURN("pliny nat. hist. 15.30", "", "")
}
```

### Data Files Required

Your custom data directory must contain these files:

- `greek_data.json` - Greek author/work mappings
- `latin_data.json` - Latin author/work mappings
- `schol_data.json` - Scholia mappings
- `other_data.json` - Other authors (e.g., Shakespeare)

You can use the files from this repository's `data/` directory or create your own following the same JSON structure.

## Output

The application generates:

- `resolved.jsonl` - Successfully resolved citations with CTS URNs
- `unresolved.jsonl` - Citations that could not be resolved (typically 0 with current implementation)

## Citation Format

Each resolved citation entry contains:

```json
{
  "n_attrib": "Soph. OT 151",
  "bibl": "O. T. 151 lyr.",
  "ref": "soph. ot 151",
  "urn": "urn:cts:greekLit:tlg0011.tlg004.perseus-grc2:151",
  "quote": "τᾶς πολυχρύσου | Πυθῶνος ἀγλαὰς ἔβας | Θήβας-",
  "xml_context": "...surrounding XML context...",
  "filename": "testdata/xml/campbell-sophlanguage-2.xml",
  "doc_cit_urn": ":citations-1.0"
}
```

## Supported Authors & Works

The application includes comprehensive mappings for ancient Greek and Latin literature.
It also resolves citations to some English authors (mainly Shakespeare)
that are frequently cited in commentaries on ancient texts.

Note that I've by and large added authors to catch everything cited
in Campbell's Sophocles grammar and Jebb's Sophocles commentaries.
Hence, for instance, coverage of Latin works and authors is a bit spottier
than Greek works and authors.

### Dynamic Author Disambiguation

The system automatically resolves ambiguous authors based on work titles:

- `plin. nh 15.30` → Pliny Senior (Naturalis Historia)
- `plin. ep. 1.1` → Pliny Junior (Epistulae)
- `seneca oed. 812` → Seneca Junior (Oedipus)
- `seneca controv. 1.1` → Seneca Senior (Controversiae)

## Architecture

### Core Components

- `cmd/citation-processor/main.go` - Main application with citation extraction and processing logic
- `pkg/resolver/resolver.go` - URN resolution, author/work mapping, and reference parsing
- `pkg/loader/data_loader.go` - Data loading, work abbreviation generation, and Latin author disambiguation

### Data Files

- `data/greek_data.json` - Greek author abbreviations, work mappings, and URN templates
- `data/latin_data.json` - Latin author abbreviations, work mappings, and URN templates
- `data/other_data.json` - Additional authors (Shakespeare, etc.)
- `data/schol_data.json` - Scholia and commentary mappings

### Test Suite

To run tests, use `go run ./cmd/citation-processor/`.

- `cmd/citation-processor/main_test.go` - Comprehensive test suite including:
  - End-to-end citation processing tests
  - Individual resolver function tests
  - Work abbreviation generation tests
  - Performance benchmarks

### Test Data

- `testdata/xml/` - Sample XML files for testing
- `testdata/expected/` - Expected output files for test validation

## Performance

Current performance on test datasets:

- **Campbell XML**: 2,004 citations resolved (100%)
- **VIAF XML**: 2,924 citations resolved (100%)
- **Total**: 4,928 citations with 0 unresolved

Since these are both works on Sophocles and I focussed on mapping citations
found in works on Sophocles, the coverage will be less complete with other
texts.

## Running Tests

```bash
# Run all tests
go test ./cmd/citation-processor/

# Run specific tests
go test ./cmd/citation-processor/ -run TestCitationProcessing

# Run benchmarks
go test ./cmd/citation-processor/ -bench=.
```

## Other Details

### Citation Extraction Modes

**Cit Tag Mode (default)**:

- Extracts citations from any XML structure containing citation patterns
- Finds `<cit>` tags enclosing `<bibl>`, and `<ref>` elements throughout the document

**No Cit Tag Mode (--nocit)**:

- Extracts `<bibl>` and `<quote>` tags
- More broadly usable, since doesn't require `<cit>` tags

### Work Abbreviation Generation

The system automatically generates multiple abbreviation variants:

- First letters: "n", "n.", "na", "nat.", etc.
- Initials: "nh" for "naturalis historia"
- Partial words: "natur", "natura", etc.
- Full forms: "natural history", "naturalis_historia"

### URN Format

Generated URNs follow CTS (Canonical Text Services) standards:

```
urn:cts:{literature}:{author}.{work}.{edition}:{passage}
```

Examples:

- `urn:cts:greekLit:tlg0011.tlg004.perseus-grc2:151`
- `urn:cts:latinLit:phi0978.phi001.perseus-lat2:15.30`

### Quirks

There are of course ambiguous citations that can't be
addressed through the resolution logic here.
For instance, _Arist. Met._ could refer to Aristotle's _Metaphysics_
or _Meteorology_. One way of handling this is simply to add
one mapping to the relevant JSON file in the `data` directory.
I've done this in several cases, especially where it strikes me
that an ambiguous abbreviation is more likely to refer
to one thing over another (in this case, "Met." is explicitly mapped to "Metaphysics").

Where this is not the case and the resolution logic can resolve the citation, e.g.
by expanding "Met." to "Metaphysics" or "Meteorology", the application's behaviour
is not wholly predictable, and may change from one run to another.
This is ultimately a consequence of Go's nondeterministic hash map implementation.

## TODO

- [ ] Add option to output distinct jsonl files for each xml file
- [ ] Keep track of ambiguous citation resolutions in separate output file or log
