# f4 PlugRing: The App Store Philosophy

PlugRing is community-driven catalog for f4 plugins, macros, and color schemes. Inspired by the legendary Far Manager PlugRing, it aims to be a single ring to rule them all, but built for the modern era.

## 1. Hosting & Submission (The "No Backend" Approach)

The entire PlugRing catalog is hosted directly in the `f4` GitHub repository. There is no proprietary database or paid hosting.

To submit or update a plugin, you simply create a Pull Request adding or modifying a Markdown file in the `plugring/` directory of the repository.

**Policy:** Open to everything except malware. No censorship, but must comply with GitHub's TOS and basic legal requirements. We strongly encourage (but do not strictly mandate) providing tests for your plugins.

## 2. The Format (Human First, Machine Second)

Every plugin in the catalog is a single `.md` file.

*   **For humans:** The body of the Markdown file contains the description, screenshots, and documentation. It is rendered natively by GitHub.
*   **For machines:** The file starts with a YAML frontmatter block containing strict metadata. A GitHub Action compiles these into a fast `index.json` for the `f4` client to consume.

### Example `plugring/f4-python-fmt.md`

```yaml
---
id: "python-fmt"
name: "Python Formatter"
version: "1.0.2"
author: "unxed"
description: "Format Python code using Black directly from the editor."
url: "https://github.com/unxed/f4-python-fmt/releases/download/v1.0.2/plugin_{os}_{arch}.zip"
entrypoint: ".venv/bin/python main.py"
setup_cmd: "python3 -m venv .venv && .venv/bin/pip install -r requirements.txt"
---