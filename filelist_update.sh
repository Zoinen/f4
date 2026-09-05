#!/bin/bash

OUTPUT="docs/FILELIST.md"

echo "# Project Structure" > "$OUTPUT"
echo "" >> "$OUTPUT"

if command -v tree &> /dev/null; then
    tree -a -I ".git|$OUTPUT" | sed 's/^/    /' >> "$OUTPUT"
else
    find . -path './.git' -prune -o -name "$OUTPUT" -prune -o -print | sed 's/^/    /' >> "$OUTPUT"
fi

echo "File list updated in $OUTPUT"
