# CI Conan binary remote

The f4 CI jobs use the Conan virtual repository on `de_zoin` as a durable
binary cache.  The server is intentionally separate from GitHub Actions
cache: Actions remains a fast per-run accelerator, while Artifactory keeps
completed Conan packages across cache-key changes and workflow retries.

Public Conan endpoints:

- Read/install: `https://v2202510307500392562.nicesrv.de:9443/artifactory/api/conan/f4-conan-virtual`
- Upload: `https://v2202510307500392562.nicesrv.de:9443/artifactory/api/conan/f4-conan-local`

The virtual repository contains the local repository and ConanCenter as its
remote source.  CI credentials are stored only in GitHub Actions Secrets; no
passwords or access tokens belong in this repository.

## Server layout

- Artifactory CE: Docker container `f4-artifactory`, persistent data in
  `/srv/jfrog/artifactory/var`.
- PostgreSQL: Docker container `f4-postgres`, persistent data in
  `/srv/jfrog/postgres`.
- Both containers use the private Docker network `f4-infra`; Artifactory is
  bound to loopback and exposed through Caddy on port `9443`.
- Caddy certificate files are in `/etc/caddy/certs`; the Certbot deploy hook
  reloads Caddy after renewal.

The Caddy configuration is kept in `Caddyfile` next to this document.  The
server also hosts unrelated services on ports 80, 443, and 8443, so the Conan
endpoint must remain on 9443 unless that allocation is deliberately changed.
