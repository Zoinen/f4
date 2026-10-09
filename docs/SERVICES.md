# Windows services panel (`plugins/svcmgr`)

[f4#311](https://github.com/unxed/f4/issues/311): a panel with the services of a Windows machine, in the spirit of Far Manager's SvcMgr. Windows only: on every other OS the plugin registers nothing.

Open it from the plugin menu or the command palette as **Services** (the entry is added automatically by the panel-provider API, like ProcList). The panel lists every Win32 service with its name, display name, state and process id; the title shows how many. The list is read from the Service Control Manager with the enumerate right only, so listing works without administrator rights.

## Keys

| Key | Action |
| --- | --- |
| F5 | Reload the list |
| Enter | Details of the service under the cursor (below) |
| Shift+F1 | Start |
| Shift+F2 | Stop (asks first: services that depend on it stop too) |
| Shift+F3 | Pause a running service, or resume a paused one |
| Shift+F4 | Change the start type: Automatic, Automatic (delayed start), Manual or Disabled (Boot and System, which are for drivers, are not offered) |
| Shift+F5 | Connect to another computer (empty name: this one) |

Starting, stopping, pausing and changing the start type need the corresponding rights on the service; without them the message says what failed and the list stays as it was.

## Details (Enter)

Name, display name, state, start type, the account the service runs as, the program it starts, the services it depends on and what happens when it fails (the recovery steps, with their delays). Every field is read with the query-configuration right only.

## Another computer

Shift+F5 asks for the name or address of a computer and shows the services of its Service Control Manager; the panel title then reads "Services on <computer>". The actions above work on that machine too, as far as the account f4 runs under is allowed to on it. Connecting to a remote machine cannot be checked in CI (only compilation and tests on stand-in data), so a report of how it behaves on a real network is welcome.
