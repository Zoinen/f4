#!/usr/bin/env bash
set -Eeuo pipefail

readonly default_artifactory_url="https://v2202510307500392562.nicesrv.de:9443"
readonly conan_read_remote="f4-conan"
readonly conan_upload_remote="f4-conan-upload"
readonly default_bootstrap_ssh_host="de_zoin"
readonly artifactory_container="f4-artifactory"
readonly bootstrap_creds_path="/srv/jfrog/artifactory/var/etc/access/bootstrap.creds"

die() {
    echo "error: $*" >&2
    exit 1
}

change_password_via_bootstrap_creds() {
    local ssh_host="${F4_ARTIFACTORY_BOOTSTRAP_SSH_HOST:-${default_bootstrap_ssh_host}}"

    command -v ssh >/dev/null 2>&1 || die "ssh is required to bootstrap the Artifactory password"
    if ! ssh -q -o BatchMode=yes -o ConnectTimeout=10 "${ssh_host}" true >/dev/null 2>&1; then
        die "cannot reach the configured Artifactory bootstrap host '${ssh_host}'; set F4_ARTIFACTORY_IDENTITY_TOKEN or use the Artifactory UI"
    fi

    echo "[FIX] Writing the one-time Access bootstrap credentials on ${ssh_host}; the password is sent over stdin and is not displayed." >&2
    {
        printf '%s\n' "${artifactory_username}"
        printf '%s\n' "${new_password}"
    } | ssh -q -o BatchMode=yes "${ssh_host}" \
        "IFS= read -r username; IFS= read -r password; umask 077; printf '%s@*=%s\\n' \"\${username}\" \"\${password}\" > '${bootstrap_creds_path}'; chmod 600 '${bootstrap_creds_path}'; chown 1030:1030 '${bootstrap_creds_path}'; docker restart '${artifactory_container}' >/dev/null; for i in \$(seq 1 300); do if curl --max-time 2 --silent --fail http://127.0.0.1:8082/router/api/v1/system/health >/dev/null 2>&1; then rm -f '${bootstrap_creds_path}'; exit 0; fi; sleep 1; done; rm -f '${bootstrap_creds_path}'; exit 1" >/dev/null \
        || die "Artifactory did not accept the bootstrap credentials or did not become healthy after restart"
}

change_password_via_identity_token() {
    echo "[FIX] Changing the Artifactory password through the existing identity token..." >&2
    if ! NEW_PASSWORD="${new_password}" python3 -c '
import json
import os
import sys

json.dump({"password": os.environ["NEW_PASSWORD"]}, sys.stdout)
' | curl --fail --silent --show-error \
        -X PUT \
        -H "Authorization: Bearer ${artifactory_identity_token}" \
        -H 'Content-Type: application/json' \
        --data-binary @- \
        "${artifactory_url}/access/api/v2/users/${artifactory_username}/password"; then
        die "Artifactory rejected the password change"
    fi
}

cleanup_secrets() {
    unset artifactory_identity_token new_password new_password_confirmation
}
trap cleanup_secrets EXIT

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

[[ -t 0 && -t 1 ]] || die "run this script from an interactive terminal"

read -r -p "Artifactory base URL [${default_artifactory_url}]: " artifactory_url
artifactory_url="${artifactory_url:-${default_artifactory_url}}"
artifactory_url="${artifactory_url%/}"

read -r -p "Artifactory username [admin]: " artifactory_username
artifactory_username="${artifactory_username:-admin}"

use_identity_token=false
if [[ -n "${F4_ARTIFACTORY_IDENTITY_TOKEN:-}" ]]; then
    artifactory_identity_token="${F4_ARTIFACTORY_IDENTITY_TOKEN}"
    use_identity_token=true
    echo "Using the existing local Artifactory authorization; it will not be displayed."
fi

read -r -s -p "New Artifactory password: " new_password
printf '\n'
read -r -s -p "Repeat new Artifactory password: " new_password_confirmation
printf '\n'
[[ "${new_password}" == "${new_password_confirmation}" ]] || die "passwords do not match"
[[ -n "${new_password}" ]] || die "new password cannot be empty"

if [[ "${use_identity_token}" == true ]]; then
    change_password_via_identity_token
else
    local_bootstrap_ssh_host="${F4_ARTIFACTORY_BOOTSTRAP_SSH_HOST:-${default_bootstrap_ssh_host}}"
    read -r -p "Restart Artifactory on ${local_bootstrap_ssh_host} to set the password? [Y/n] " bootstrap_confirmation
    if [[ -n "${bootstrap_confirmation}" && ! "${bootstrap_confirmation}" =~ ^[Yy]$ ]]; then
        die "Artifactory password bootstrap was not requested"
    fi
    change_password_via_bootstrap_creds
fi
echo "Artifactory password changed."

if ! command -v conan >/dev/null 2>&1; then
    echo
    echo "Conan 2 is not installed, so the Artifactory password was set but Conan remotes were not configured."
    echo "Install Conan 2 and run the following commands when it is available:"
    echo "  conan remote add ${conan_read_remote} ${artifactory_url}/artifactory/api/conan/f4-conan-virtual --force"
    echo "  conan remote add ${conan_upload_remote} ${artifactory_url}/artifactory/api/conan/f4-conan-local --force"
    echo "  conan remote login ${conan_read_remote} ${artifactory_username}"
    echo "  conan remote login ${conan_upload_remote} ${artifactory_username}"
    exit 0
fi

conan remote add "${conan_read_remote}" \
    "${artifactory_url}/artifactory/api/conan/f4-conan-virtual" \
    --force
conan remote add "${conan_upload_remote}" \
    "${artifactory_url}/artifactory/api/conan/f4-conan-local" \
    --force

echo "Logging Conan into the read remote..."
conan remote login "${conan_read_remote}" "${artifactory_username}" \
    --password "${new_password}"
echo "Logging Conan into the upload remote..."
conan remote login "${conan_upload_remote}" "${artifactory_username}" \
    --password "${new_password}"

echo "Checking access to the f4 Conan virtual repository..."
conan list 'qt/6.11.1:*' -r "${conan_read_remote}" -f compact >/dev/null
echo "Conan read access is working."

read -r -p "Conan package pattern to upload now (empty to skip): " upload_pattern
if [[ -n "${upload_pattern}" ]]; then
    echo "The following pattern will be uploaded to ${conan_upload_remote}:"
    echo "  ${upload_pattern}"
    read -r -p "Continue? [y/N] " upload_confirmation
    if [[ "${upload_confirmation}" =~ ^[Yy]$ ]]; then
        conan upload "${upload_pattern}" \
            -r "${conan_upload_remote}" \
            --confirm --check
        echo "Conan package upload completed."
    else
        echo "Package upload skipped."
    fi
else
    echo "Package upload skipped."
fi

echo
echo "Done. Conan remotes configured:"
conan remote list | sed -n "/${conan_read_remote}/p; /${conan_upload_remote}/p"
