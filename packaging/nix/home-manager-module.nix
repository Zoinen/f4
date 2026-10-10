{ config, lib, pkgs, ... }:

# Home Manager module for f4. Declared keys are merged into f4's live
# profile files at every `home-manager switch` rather than linked into
# place: f4 reads these files at start and rewrites them at runtime (its
# settings and its session state share settings.ini), so a store link would
# be replaced by f4's first atomic write and collide with the next switch.
# See programs.f4.settings for what the merge means for edits made in f4.
let
  inherit (lib) mkEnableOption mkIf mkMerge mkOption types;

  cfg = config.programs.f4;

  ini = import ./ini.nix { inherit pkgs; };

  # f4's per-user profile is os.UserConfigDir()/f4 everywhere except macOS,
  # where os.UserConfigDir is ~/Library/Application Support. Portable mode
  # (f4.ini next to the binary) points the profile elsewhere and is not
  # covered here.
  profileDir =
    if pkgs.stdenv.hostPlatform.isDarwin then
      "${config.home.homeDirectory}/Library/Application Support/f4"
    else
      "${config.xdg.configHome}/f4";

  managedFiles = lib.filterAttrs (_: sections: sections != { }) {
    "settings.ini" = cfg.settings;
    "hotkeys.ini" = cfg.hotkeys;
    "keymap.ini" = cfg.keymap;
    "highlight.ini" = cfg.highlight;
  };
in
{
  options.programs.f4 = {
    enable = mkEnableOption "f4, a TUI file manager reproducing the UX of far2l and Far Manager";

    package = lib.mkPackageOption pkgs "f4" {
      extraDescription = "Needs this flake's overlay (or nixpkgs' f4) to have a `pkgs.f4` to default to.";
    };

    settings = mkOption {
      type = ini.type;
      default = { };
      example = {
        Interface = {
          ColorStyle = "Radiola";
          WorkspaceTabMode = "multiple";
        };
        Panel.ShowHiddenFiles = false;
        System.ConfirmExit = false;
      };
      description = ''
        The main configuration: `settings.ini` in f4's per-user profile
        (`~/.config/f4` on Linux and other Unix, `~/Library/Application
        Support/f4` on macOS).

        Sections and keys are the ones f4 itself reads and writes -
        `[Interface]`, `[Panel]`, `[System]`, `[Editor]`, `[Viewer]` and the
        rest. `f4:config` (Commands > Configuration editor) lists every key
        with the value currently in effect. Booleans are written as `1`/`0`,
        the spelling the file uses; other enumerations are their string
        values (`WorkspaceTabMode = "multiple"`).

        f4 reads the file at start and rewrites it at runtime - settings and
        state live in the same file - so Home Manager does not own it. On
        every `home-manager switch` the keys declared here are written into
        the live file (replaced where present, appended where not) and
        nothing else in it is touched. A value changed through f4's own
        settings UI therefore reverts to the declared one on the next
        switch; a key or section removed from these options stays in the
        file until it is deleted there by hand.
      '';
    };

    keymap = mkOption {
      type = ini.type;
      default = { };
      example = {
        Common = {
          CtrlAltO = "CtrlO";
          Alt1 = "F1";
        };
      };
      description = ''
        Key remapping (`keymap.ini`): substitute one key for another before
        anything in f4 sees it, for multiplexers that claim chords upstream
        and keyboards without an F-row. Sections are area names (`Common`
        applies everywhere); a rule maps the spelling shown by the Hotkey
        Configurator to the spelling f4 should see instead.

        f4 writes a commented sample of this file on first start and never
        rewrites it afterwards. The rules declared here are merged into the
        live file with the same semantics as `settings`; see there.
      '';
    };

    hotkeys = mkOption {
      type = ini.type;
      default = { };
      example = {
        Common.AltIns = "None";
      };
      description = ''
        Hotkey bindings (`hotkeys.ini`): area -> key -> action name
        (`App.ScreenGrab`, `App.ConfigEditor`, ... - the names f4's hotkey
        settings dialog works with), or `"None"` to unbind a key.

        f4's hotkey dialog rewrites this file, so the bindings declared here
        are merged into the live file with the same semantics as `settings`;
        see there.
      '';
    };

    highlight = mkOption {
      type = ini.type;
      default = { };
      example = {
        Highlight_100 = {
          Name = "Archives";
          Mask = "*.zip, *.rar, *.7z";
          ExcludeAttributes = "Directory";
          NormalColor = "foreground:#FF00FF | background:#000000";
          Group = 3;
        };
      };
      description = ''
        File highlighting and sort groups (`highlight.ini`). `[Highlight_N]`
        sections match panel items by mask, attributes, size or date and
        paint and/or group them; f4 orders the rules by the number `N`. The
        commented sample f4 writes on first start documents the keys (Name,
        Mask, IncludeAttributes/ExcludeAttributes, NormalColor,
        SelectedColor, CursorColor, SelectedCursorColor, Group,
        ContinueProcessing, ...).

        f4 writes that sample only when the file is missing and reads the
        file at start - restart f4 after a switch. The sections declared
        here are merged into the live file with the same semantics as
        `settings`; see there.
      '';
    };
  };

  config = mkIf cfg.enable (mkMerge [
    { home.packages = [ cfg.package ]; }

    (mkIf (managedFiles != { }) {
      home.activation.f4-profile = lib.hm.dag.entryAfter [ "writeBoundary" ] (
        ''
          mkdir -p ${lib.escapeShellArg profileDir}
        ''
        + lib.concatStrings (
          lib.mapAttrsToList (name: sections: ''
            ${ini.upsert}/bin/f4-ini-upsert \
              ${lib.escapeShellArg "${profileDir}/${name}"} \
              ${pkgs.writeText name (ini.render sections)}
          '') managedFiles
        )
      );
    })
  ]);
}
