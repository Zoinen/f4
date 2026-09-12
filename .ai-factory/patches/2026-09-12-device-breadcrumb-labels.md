# Separate device breadcrumb labels from navigation addresses

## Problem

Android's manager exposed a raw URI as its display title, mounted Android
FISH+ views inherited a network icon, and device breadcrumbs printed URI schemes.

## Root Cause

The transport identity, navigation address, and display label are independent
contracts. Reusing their strings or inheriting the transport's icon conflates
them when a plugin is layered over a generic backend.

## Solution

Keep canonical device URIs for navigation and editing. Allow the shared path
control's root label to be overridden by the host without replacing its path
segments. Publish Android manager and mounted-device presentation through the
existing VFS title/icon interfaces, preserving generic network FISH+ behavior.

## Prevention

Regression coverage checks manager and child paths, both device families,
root/child clicks, canonical edit fields, and actual text/icon/separator leaves
at DPR 1.75 with a rendered capture. Further coverage can exercise narrow
windows and translated root titles without changing URI routing.

The native edit field initially failed at (206.5, 79.25) physical pixels for
Android and (159.25, 79.25) for iOS. Apply the shared scene-space pixel correction
to the actual TextInput, preserving unit transforms as well as snapped origins.

## Tags

`#qml` `#vfs` `#breadcrumbs` `#icons` `#regression-first`
