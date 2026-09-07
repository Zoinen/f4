# Shell Scripting with `vtui-dialog`

`vtui-dialog` is a command-line tool that brings stateful, desktop-class Far Manager / Turbo Vision style dialogs to shell scripts (Bash, Zsh, Sh, PowerShell, Fish).

It works similarly to classic tools like `dialog`, `whiptail`, `zenity`, or `kdialog`, but runs on the modern, GPU-capable `vtui` engine with full mouse, keyboard, and constraint layout support.

---

## Installation & Compilation

```bash
# Build the binary
go build -o vtui-dialog ./cmd/vtui-dialog

# (Optional) Move to your PATH
sudo cp vtui-dialog /usr/local/bin/
```

---

## Quick Examples

### 1. Message Box (`--msgbox`)

```bash
vtui-dialog --title="Backup Complete" --msgbox="Successfully backed up 1,420 files in 3.4 seconds."
```

### 2. Yes/No Question (`--yesno`)

Returns exit code `0` on "Yes", `1` on "No" or Escape.

```bash
if vtui-dialog --title="Confirmation" --yesno "Do you want to deploy to production?"; then
    echo "Deploying..."
else
    echo "Deployment aborted."
fi
```

### 3. Text Input Box (`--inputbox`)

Prints the entered text to `stdout`.

```bash
NAME=$(vtui-dialog --title="User Setup" --inputbox "Enter your full name:" "Linus Torvalds")
if [ $? -eq 0 ]; then
    echo "Welcome, $NAME!"
fi
```

### 4. Password Input Box (`--passwordbox`)

Masks characters with asterisks and prints the entered secret to `stdout`.

```bash
TOKEN=$(vtui-dialog --title="Authentication" --passwordbox "Enter API token:")
```

### 5. Menu Selection (`--menu`)

```bash
BRANCH=$(vtui-dialog --title="Git Checkout" --menu "Choose branch" \
    main "Production main branch" \
    dev  "Active development branch" \
    test "Staging environment")

if [ $? -eq 0 ]; then
    git checkout "$BRANCH"
fi
```

### 6. File & Directory Browsers (`--filebox` / `--dirbox`)

Full Far Manager style interactive browser dialog with search and navigation:

```bash
TARGET_FILE=$(vtui-dialog --title="Select File" --filebox ~/Documents)
TARGET_DIR=$(vtui-dialog --title="Select Destination" --dirbox /var/log)
```

---

## Declarative `.vui` Forms with JSON Output

You can design full forms with inputs, checkboxes, and radio buttons in `.vui` format and read all results in a single shell command as JSON:

### `user_form.vui`
```json
{
  "vuiVersion": 1,
  "root": {
    "type": "Dialog",
    "id": "userDlg",
    "props": { "title": " User Settings ", "autoSize": true, "center": true },
    "layout": { "type": "VBox", "spacing": 1, "margins": [1, 2, 1, 2] },
    "children": [
      {
        "type": "Group",
        "layout": { "type": "Form", "spacing": 1 },
        "children": [
          { "type": "Label", "props": { "text": "&Username:", "buddy": "username" } },
          { "type": "Edit", "id": "username", "props": { "text": "root" } },
          { "type": "Label", "props": { "text": "&Cluster:", "buddy": "cluster" } },
          { "type": "ComboBox", "id": "cluster", "props": { "text": "eu-central-1", "items": ["us-east-1", "eu-central-1", "ap-southeast-1"] } }
        ]
      },
      { "type": "Checkbox", "id": "debugMode", "props": { "text": "Enable &Debug Tracing", "state": 1 } },
      {
        "type": "Group",
        "layout": { "type": "HBox", "spacing": 2, "align": "center" },
        "children": [
          { "type": "Button", "id": "saveBtn", "props": { "text": "&Save", "default": true, "command": 0 } },
          { "type": "Button", "id": "cancelBtn", "props": { "text": "&Cancel", "command": 1 } }
        ]
      }
    ]
  }
}
```

### Shell Script (using `jq`):
```bash
CONFIG=$(vtui-dialog --vui=user_form.vui --json)

if [ $? -eq 0 ]; then
    USER=$(echo "$CONFIG" | jq -r '.values.username')
    REGION=$(echo "$CONFIG" | jq -r '.values.cluster')
    DEBUG=$(echo "$CONFIG" | jq -r '.values.debugMode')
    echo "Configured user=$USER in $REGION (debug=$DEBUG)"
fi
```

---

## Hardware-Accelerated GUI Rendering in Shell Scripts

Just add `--backend=gogpu`, `--backend=x11`, `--backend=wayland`, or `--backend=ebiten`:

```bash
vtui-dialog --backend=gogpu --title="Hardware Accelerated" --msgbox="Crisp TrueColor text rendered directly on the GPU!"
```
