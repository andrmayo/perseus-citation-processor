# Test Data Directory

This directory contains XML test files and expected outputs for validating the Perseus Citation Linker functionality.

## Directory Structure

```
testdata/
├── xml/                    # Input XML files for testing
│   ├── campbell-sophlanguage-2.xml
│   └── viaf2603144.xml
└── expected/               # Expected output files for validation
    ├── campbell-sophlanguage-2_resolved.jsonl
    ├── campbell-sophlanguage-2_unresolved.jsonl
    ├── viaf2603144_resolved.jsonl
    └── viaf2603144_unresolved.jsonl
```

## Test Files

### XML Input Files
- **campbell-sophlanguage-2.xml** - Campbell XML with 2,004 citations
- **viaf2603144.xml** - VIAF XML with 2,924 citations

### Expected Output Files
- **\*_resolved.jsonl** - Expected successfully resolved citations with CTS URNs
- **\*_unresolved.jsonl** - Expected unresolved citations (currently empty - 100% resolution)

## Running Tests

### Using Go Test Suite

Run the comprehensive test suite from the project root:

```bash
# Run all tests
go test ./cmd/citation-processor/

# Run citation processing tests specifically
go test ./cmd/citation-processor/ -run TestCitationProcessing

# Run with verbose output
go test ./cmd/citation-processor/ -v

# Run benchmarks
go test ./cmd/citation-processor/ -bench=.
```

### Manual Testing

Process XML files directly and compare with expected outputs:

```bash
# Process both test files with comprehensive extraction
go run cmd/citation-processor/main.go -input testdata/xml/ -cit -output test_output/

# Compare results (should show no differences)
diff test_output/campbell-sophlanguage-2_resolved.jsonl testdata/expected/campbell-sophlanguage-2_resolved.jsonl
diff test_output/viaf2603144_resolved.jsonl testdata/expected/viaf2603144_resolved.jsonl
```

### Expected Test Results

Current test performance:
- **Campbell XML**: 2,004 citations resolved (100%)
- **VIAF XML**: 2,924 citations resolved (100%)
- **Total**: 4,928 citations with 0 unresolved
- **Resolution Rate**: 100%

## Test Validation

The test suite validates:

1. **Citation Count Accuracy** - Ensures exact number of citations are extracted
2. **URN Resolution** - Verifies all citations resolve to valid CTS URNs
3. **Output Format** - Confirms JSONL structure and required fields
4. **Zero Unresolved** - Validates 100% resolution rate
5. **Deduplication** - Ensures no duplicate citations in output

## Regenerating Expected Files

If you modify the citation processing logic and need to update expected outputs:

```bash
# Clean existing expected files
rm testdata/expected/*.jsonl

# Process XML files and generate new expected outputs
go run cmd/citation-processor/main.go -input testdata/xml/ -cit -output testdata/expected/

# Verify new outputs look correct before committing
head -5 testdata/expected/campbell-sophlanguage-2_resolved.jsonl
wc -l testdata/expected/*.jsonl
```

## Test Data Sources

- **Campbell XML** - Sample from Campbell's Sophocles language analysis
- **VIAF XML** - Virtual International Authority File citation data

Both files contain diverse citation patterns testing the full range of the citation resolution system.