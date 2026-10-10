# Folder bookmarks

Ten numbered slots (`Ctrl+Shift+<digit>` saves the active panel's location, `RCtrl+<digit>` or `Ctrl+Alt+<digit>` jumps to it; the bookmarks dialog lists them). The file is far2l's `bookmarks.ini`, so it can be shared with far2l unchanged.

## What a slot remembers

* **Path**: the location of the active panel as its VFS reports it. For the local disk that is the directory; for an archive, `sftp://`, `smb://` and the like it is the URI the panel can be reopened from; for the Docker, Kubernetes and MongoDB panels it is `docker:///<container>/…`, `k8s:///<namespace>/<pod>/<container>/…`, `mongo:///<database>/<collection>` (each plugin registers a URI provider, see [DOCKER.md](DOCKER.md), [KUBERNETES.md](KUBERNETES.md), [MONGODB.md](MONGODB.md)). Folder history and saved sessions use the same addresses.
* **Plugin / PluginData** (f4#1669): when a panel plugin covers the folder (Git status, the process list, ...), the slot also records the plugin's panel provider as `Plugin=f4-panel:<ID>` and the panel's own state (filter, selected item) in `PluginData`. Jumping to the slot goes to the folder and opens that plugin over it; if the plugin is not available (disabled, not installed here) only the folder is opened and a message says so. far2l's own `Plugin` values are kept as they are and never mistaken for f4's.

## The dialog

Each row shows the slot digit and the path, and `[ID]` after it for a plugin bookmark. `Ins` or `Ctrl+N` saves the current location, `Del` clears a slot, `Shift+Up`/`Shift+Down` move it, `F4` edits the path: for a plugin bookmark the edit box names the plugin in its title, and changing the path moves the bookmark while it keeps opening the same plugin (`Del` and saving again drops the plugin).

## Drive-menu links

The named links of the drive menu (Alt+F1/Alt+F2; `drive-bookmarks.ini`) store a path only, in the same form as above, so a link made on a Docker, Kubernetes or MongoDB panel (or an `sftp://` one) brings that panel back. In the drive menu, Ctrl+Up and Ctrl+Down move the link under the cursor one place up or down, and the order is saved to the file. The same keys move a tool row (a plugin such as AI, Android or Network) among the tool rows; that order is kept one name per line in `settings/drive-tools-order.txt` of the profile, and applies while the "sort plugins by hotkey" option is off.
