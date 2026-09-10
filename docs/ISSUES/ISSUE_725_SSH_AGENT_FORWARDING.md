# Issue #725 solution review

## Problem

NetFox used the local SSH agent for authentication and then unconditionally
forwarded that agent to the remote server. Both the shared SSH dialer and the
FISH+ session path could expose the user's signing capability to remote root.

## Candidate solutions

1. Keep forwarding and add a confirmation prompt. This would make a security
   decision at connection time, complicate non-interactive SFTP/reconnects,
   and still make the unsafe behavior easy to approve accidentally.
2. Remove agent use entirely. This avoids forwarding but unnecessarily loses
   the normal OpenSSH behavior where a local agent is only an authentication
   source.
3. Keep the agent as a local `ssh.PublicKeysCallback`, remove transport and
   session forwarding, and close the local agent socket after authentication.
   This matches OpenSSH's default and keeps SFTP/FISH+ authentication intact.

## Chosen solution

Use the local agent only while `ssh.NewClientConn` performs authentication.
Do not call `agent.ForwardToAgent` from `DialSSH` or
`agent.RequestAgentForwarding` from the FISH+ session path. Close the local
agent connection after the handshake. Add an integration SSH server that
records both global agent-forwarding requests and FISH+ session requests.

## Verification

- Test that a real local agent socket is used without forwarding.
- Test that FISH+ does not request forwarding when `SSH_AUTH_SOCK` is set.
- Run the full NetFox and project test suites, vet, and native Windows
  validation.

— zoin-bot
