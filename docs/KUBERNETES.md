# Kubernetes panel (`plugins/k8sfs`)

[f4#1663](https://github.com/unxed/f4/issues/1663), Kubernetes part: a cluster as a drive.

Open it from the drive menu (Alt+F1) as **Kubernetes**. Namespaces are the top-level folders, then pods, then a pod's containers; inside a container is its file system. F3 and F5 work as on any panel, and files inside a container can be written (see below).

## How

* **kubectl-free.** The API server is spoken to directly with `net/http`: `GET /api/v1/namespaces` and `/api/v1/namespaces/{ns}/pods` for the tree. No client-go (a huge dependency tree) and no `kubectl` binary.
* **Files through exec.** The API has no file endpoint, so, like `kubectl exec` and `kubectl cp`, the plugin runs commands in the container over the exec WebSocket (`v4.channel.k8s.io`, via `golang.org/x/net/websocket`, which f4 already depends on): `ls -1ApL` for names and folder flags, `stat -c` for sizes, times and modes, `cat` for content (copied to a temporary file so F3/F5 get random access). The container needs `ls`, `stat` and `cat`; busybox has them all. Images without them (distroless) can be browsed down to the container but not into it.
* **Credentials from a kubeconfig** (`KUBECONFIG`, first file, or `~/.kube/config`): the current context's server, CA (file or inline), bearer token (or `tokenFile`), client certificate and key. Users whose credentials come from a helper program (`exec:` plugins such as `gke-gcloud-auth-plugin` or `aws eks get-token`) are supported: the helper is run once when the panel opens (with `KUBERNETES_EXEC_INFO`, no stdin), and the token or client certificate it prints is used; its own error message is shown if it fails. The removed `auth-provider` mechanism is refused with a message saying so; nothing is sent without credentials.
* Nothing is read or connected until the panel is opened.

## Writing

The exec protocol available over a plain WebSocket (`v4.channel.k8s.io`) cannot close a command's stdin, so `kubectl cp`'s way (`tar xf -`) is not possible. Instead:

* **Copy in** sends the file as base64 in the arguments of short commands, 12 KiB per command (`sh -c 'printf %s "$1" | base64 -d >> "$2"'`, the first one truncating). Every chunk is one exec round trip, so files are capped at 8 MiB; a larger one is refused with a hint to use `kubectl cp`.
* **mkdir, delete, rename** are single `mkdir`, `rm -rf` and `mv` commands (within one container).
* The container needs `sh`, `base64`, `mkdir`, `rm` and `mv`; busybox has them all. Attributes, and namespaces/pods/containers themselves, are not changed from the panel.

* **One drive per context.** The plain **Kubernetes** entry uses the current context. Every context of the kubeconfig also gets its own entry in the drive menu, `Kubernetes (name)`, read when f4 starts (a context added while f4 runs shows up after a restart); it connects to that context's cluster with that context's credentials, including exec helpers.
* **In a pod.** With no kubeconfig at all, inside a pod (`KUBERNETES_SERVICE_HOST` set), the pod's service account is used: its token and CA from `/var/run/secrets/kubernetes.io/serviceaccount`.
* **Addresses.** The panel path is `k8s:///<namespace>/<pod>/<container>/<path>`, so bookmarks, folder history and saved sessions (f4#1669) bring the panel back through the `k8s://` URI provider; the cluster is the one of the current kubeconfig. A context's panel is `k8s://<context>/<namespace>/<pod>/<container>/<path>`, the context name escaped as in a URL.

## Not yet

Files over 8 MiB (needs a stdin-capable exec, `v5.channel.k8s.io`), refreshing an expiring exec token, merging several KUBECONFIG files, ephemeral/init containers, and the lite build (full build only, like the Docker panel).
