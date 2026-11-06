# macOS local shell login environment

## Problem and root cause

The persistent panel shell was interactive but not a login shell. When f4 was
launched from the Dock, its sparse inherited PATH was not augmented by the
user's login profile. Commands could resolve a different Python interpreter
and fail to import packages available in other macOS terminals.

## Fix

Use Darwin-only login arguments for the persistent local shell. Leave generic
PTY launches, noninteractive commands, and other platforms unchanged. Let the
shell interpret its own startup files instead of sourcing files or injecting
Homebrew paths in f4.

## Regression and prevention

An actual zsh PTY test using arguments captured from panel startup failed before
the fix (`off:on:-rc`) and passed after it (`on:on:profile-rc`). Test shell startup
with an isolated login profile and minimal inherited PATH; an already configured
developer terminal can otherwise conceal this class of bug.
