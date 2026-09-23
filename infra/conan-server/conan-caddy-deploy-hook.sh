#!/bin/sh
set -eu

cert_dir=/etc/caddy/certs
install -d -m 0750 -o root -g caddy "$cert_dir"
install -m 0644 -o root -g caddy "$RENEWED_LINEAGE/fullchain.pem" "$cert_dir/conan-fullchain.pem"
install -m 0640 -o root -g caddy "$RENEWED_LINEAGE/privkey.pem" "$cert_dir/conan-privkey.pem"
systemctl reload caddy
