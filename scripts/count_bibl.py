#!/usr/bin/env python
import sys
import xml.etree.ElementTree as ET

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python count_bibl.py <xml_file>", file=sys.stderr)
        sys.exit(1)

    xml_file = sys.argv[1]

    try:
        tree = ET.parse(xml_file)
        root = tree.getroot()

        # Extract namespace from root element if it exists
        namespace = root.tag.split("}")[0].strip("{") if "}" in root.tag else ""

        # Find all <cit> elements regardless of where they are in the tree
        if namespace:
            cit_elements = root.findall(".//{%s}bibl" % namespace)
        else:
            cit_elements = root.findall(".//bibl")

        print(len(cit_elements))

    except FileNotFoundError:
        print(f"Error: File '{xml_file}' not found", file=sys.stderr)
        sys.exit(1)
    except ET.ParseError as e:
        print(f"Error: Failed to parse XML file: {e}", file=sys.stderr)
        sys.exit(1)
