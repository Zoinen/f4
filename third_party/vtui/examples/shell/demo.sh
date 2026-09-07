#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$DIR/../../vtui-dialog"

if [ ! -f "$BIN" ]; then
    echo "Compiling vtui-dialog..."
    (cd "$DIR/../.." && go build -o vtui-dialog ./cmd/vtui-dialog)
fi

# 1. Welcome Message
"$BIN" --title=" Welcome " --msgbox "Welcome to the vtui shell scripting demo!"

# 2. Text Input
NAME=$("$BIN" --title=" User Identification " --inputbox "Please enter your name:" "Explorer")
if [ $? -ne 0 ] || [ -z "$NAME" ]; then
    echo "Cancelled."
    exit 0
fi

# 3. Menu Selection
ACTION=$("$BIN" --title=" Tasks " --menu "What would you like to do, $NAME?" \
    status "Check system status" \
    file   "Choose a file to inspect" \
    quit   "Exit demo")

case "$ACTION" in
    status)
        "$BIN" --title=" System Status " --msgbox "OS: $(uname -s)\nArch: $(uname -m)\nHost: $(hostname)"
        ;;
    file)
        CHOSEN=$("$BIN" --title=" Select a File " --filebox "$HOME")
        if [ -n "$CHOSEN" ]; then
            "$BIN" --title=" Selected " --msgbox "You selected:\n$CHOSEN"
        fi
        ;;
    quit)
        exit 0
        ;;
esac

echo "Demo finished successfully."
