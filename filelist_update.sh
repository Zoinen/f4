#!/bin/bash

# Name of the output file
OUTPUT="filelist.md"

# Create or overwrite the file with a header
echo "# Project Structure" > "$OUTPUT"
echo "Last updated: $(date '+%Y-%m-%d %H:%M:%S')" >> "$OUTPUT"
echo "" >> "$OUTPUT"
echo "\`\`\`text" >> "$OUTPUT"

# Check if the 'tree' utility is installed
if command -v tree &> /dev/null; then
    # Use 'tree' if available (ignoring .git and the output file itself)
    tree -a -I ".git|$OUTPUT" >> "$OUTPUT"
else
    # Fallback using 'find' if 'tree' is not installed
    # Exclude .git and the output file from the output
    find . -path './.git' -prune -o -name "$OUTPUT" -prune -o -print >> "$OUTPUT"
fi

echo "\`\`\`" >> "$OUTPUT"

echo "File list updated in $OUTPUT"
