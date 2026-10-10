# Docker panel (`plugins/dockerfs`)

[f4#1663](https://github.com/unxed/f4/issues/1663), Docker part: containers as a drive.

Open it from the drive menu (Alt+F1) as **Docker**. The top level lists all containers (running or not) as folders; inside one is that container's file system. F3, F5 and Enter work as on any panel, in both directions:

* **Copy into a container** (F5/F6 onto the panel) and **mkdir** upload a tar with `PUT /containers/{id}/archive`, the way `docker cp` does. They work on stopped containers too.
* **Delete and rename** have no Engine API call, so they run `rm -rf` and `mv` inside the container (`POST /containers/{id}/exec`, no shell). That needs a *running* container with those tools in it (not a distroless image); otherwise the error says the command failed.
* Not supported: changing attributes, and anything on the container list itself (creating, removing or renaming containers).

## Why this shape

* **Built-in Go plugin, standard library only.** The Docker Engine speaks plain HTTP on a unix socket (or `tcp://`). Linking the Docker SDK would add a large dependency tree for four endpoints, and shelling out to the `docker` CLI would need the CLI to be installed. Neither is needed: `net/http` with a unix-socket dialer is enough, so the plugin adds no dependency, needs no CGO and no external tool. It follows `plugins/sqlite` and `plugins/ios` (in-process plugin, `vfs.VFS` implementation, registered with the host).
* **One VFS, POSIX paths.** `/` lists containers, `/<container>` is a container's root, the rest is the path inside it. Containers are looked up by name (the short id for an unnamed one).
* **Reading via the archive endpoint** (`/containers/{id}/archive`, what `docker cp` uses): `HEAD` gives a path's stat in a header, `GET` gives a tar. It works on stopped containers too and needs nothing inside the image (no shell, no `ls`). Symlinks (`/bin -> usr/bin`) are followed by the plugin, so they can be entered like folders.
* **Files are copied out to a temporary file** on `Open`, because the endpoint only streams and F3/F5 need random access.

## Connection

The plugin keeps no credentials of its own: it reads the places the `docker` CLI reads, so a TLS key stays in files the user already protects and nothing secret is typed into f4 or written to its settings. Order, as in the CLI: `DOCKER_CONTEXT`, then `DOCKER_HOST`, then the current context of `~/.docker/config.json` (`docker context use`; the directory is `DOCKER_CONFIG` if set), then the local daemon.

* `DOCKER_HOST`: `unix:///path/to.sock`, `tcp://host:port` or `https://host:port`. Unset: `/var/run/docker.sock`, then the rootless `$XDG_RUNTIME_DIR/docker.sock`. On Windows the default is Docker Desktop's named pipe (`npipe:////./pipe/docker_engine`, also accepted in `DOCKER_HOST`); it is opened as a plain file and served by a small one-request-per-connection transport, because a synchronous pipe handle cannot read and write at once the way `net/http` does.
* **TLS.** `DOCKER_TLS_VERIFY` (any value) or an `https://` address turns it on; `ca.pem` (trusted CA; the system roots without it), `cert.pem` and `key.pem` (client certificate) are read from `DOCKER_CERT_PATH` or the configuration directory. A certificate file that does not parse is an error, never silently skipped. TLS 1.2 or newer.
* **Contexts.** A context created by `docker context create` is read from the CLI's store (`contexts/meta/<sha256 of the name>/meta.json`, TLS files in `contexts/tls/<same>/docker/`): its host, its `ca.pem`/`cert.pem`/`key.pem`, and `SkipTLSVerify`. Like the CLI, a context uses TLS when it has TLS files or skips verification, and is used as written otherwise. A context that does not exist, or has no docker endpoint, is reported by name.
* **One drive per context.** Every context of the store that has a docker endpoint also gets its own entry in the drive menu, `Docker (name)`, read when f4 starts; it talks to that context's daemon whatever `DOCKER_HOST` says. The plain **Docker** entry follows the order above. (A context created while f4 runs shows up after a restart.)
* Not supported yet: `ssh://` (also in a context). Nothing connects until a panel is opened.

* **Addresses.** The panel path is `docker:///<container>/<path>`, so bookmarks, folder history and saved sessions (f4#1669) bring the panel back through the `docker://` URI provider; the server is the one the drive menu entry uses (see above). A context's panel is `docker://<context>/<container>/<path>`, the context name escaped as in a URL.

## Limits of this part

* Delete/rename need a running container with `rm`/`mv`; the reason of a failure is only the exit status.
* Uploads go through a temporary file (a tar header needs the size up front).
* The archive endpoint has no "one level only" mode: listing a folder streams the tar of everything under it. Listing `/` of a big image is slow, and after 400000 entries the listing stops and says it is partial.
* The full build only. The lite build (see `internal/plughost/plugins_lite.go`) does not include it yet.

## Next parts

Kubernetes (pods and containers through the API server, same panel shape) and MongoDB (databases and collections as folders and files), each as its own plugin in its own part.
