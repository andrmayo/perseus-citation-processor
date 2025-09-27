# Perseus Citation Linker - Go Version

A comprehensive Go implementation of the Perseus citation processing pipeline for extracting and resolving citations from XML documents to CTS URNs (Canonical Text Services Uniform Resource Names).

## Features

- **Flexible Citation Extraction**: Supports two extraction modes:
  - `<cit>` tag mode: Comprehensive extraction from any XML structure containing citation patterns
  - `<bibl>` tag mode: Traditional extraction focused on bibliographic references
- **Advanced Reference Resolution**: Resolves author/work abbreviations with dynamic disambiguation (e.g., Pliny Senior vs Junior)
- **Work Abbreviation Generation**: Automatically generates multiple abbreviation variants for work titles
- **CTS URN Generation**: Creates standardized URNs following Canonical Text Services format
- **Complete Coverage**: Achieves 100% citation resolution on test datasets
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

The application includes comprehensive mappings for ancient Greek and Latin literature:

### Greek Authors
- **Major Tragedians**: Aeschylus, Sophocles, Euripides
- **Epic Poets**: Homer, Hesiod, Apollonius Rhodius
- **Historians**: Herodotus, Thucydides, Xenophon, Diodorus Siculus
- **Philosophers**: Plato, Aristotle, Plutarch
- **Orators**: Demosthenes, Aeschines, Dinarchus
- **Others**: Aristophanes, Pindar, Apollodorus, Aelian, Greek Anthology

### Latin Authors
- **Epic/Poetry**: Vergil, Ovid, Horace, Catullus, Juvenal, Statius
- **Prose**: Caesar, Cicero, Livy, Tacitus, Sallust
- **Philosophy/Letters**: Seneca (Senior & Junior with automatic disambiguation)
- **Natural History**: Pliny (Senior & Junior with automatic disambiguation)
- **Comedy**: Plautus, Terence
- **Biography**: Suetonius
- **Others**: Lucretius, Propertius, Tibullus, Valerius Flaccus

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

## Running Tests

```bash
# Run all tests
go test ./cmd/citation-processor/

# Run specific tests
go test ./cmd/citation-processor/ -run TestCitationProcessing

# Run benchmarks
go test ./cmd/citation-processor/ -bench=.
```

## Contributing

To add support for new authors or works:

1. **Greek authors**: Add entries to `data/greek_data.json`
2. **Latin authors**: Add entries to `data/latin_data.json`
3. **Work abbreviations**: The system automatically generates common abbreviation patterns
4. **Custom disambiguation**: Implement logic similar to the Pliny/Seneca disambiguation in `pkg/loader/data_loader.go`

## Technical Details

### Citation Extraction Modes

**Comprehensive Mode (`-cit` flag)**:
- Extracts citations from any XML structure containing citation patterns
- Finds `<cit>`, `<bibl>`, and `<ref>` elements throughout the document
- Works with any XML document structure
- Recommended for maximum citation coverage

**Traditional Mode (default)**:
- Extracts only standalone `<bibl>` tags
- More conservative extraction approach

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