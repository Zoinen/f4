{ pkgs, home-manager }:

# `checks.<system>.home-manager-module`: the tests behind the Home Manager
# module. Three things are verified:
#
#   1. ini.render writes values the way f4's files spell them;
#   2. f4-ini-upsert merges declared keys into a live file without touching
#      anything else in it (and is idempotent, creates a missing file,
#      refuses malformed input);
#   3. the module evaluates under real Home Manager - options, types and
#      the activation entry all merge - without building f4 itself.
let
  inherit (pkgs) lib;

  ini = import ./ini.nix { inherit pkgs; };

  sampleIni = {
    Interface = {
      ColorStyle = "Radiola";
      Num = 3;
      ShowHidden = false;
    };
    Panel = {
      Mask = "*.zip, *.7z";
      Color = "foreground:#FF00FF | background:#000080";
    };
  };

  hm = home-manager.lib.homeManagerConfiguration {
    inherit pkgs;
    modules = [
      ./home-manager-module.nix
      {
        home.username = "hm";
        home.homeDirectory = "/home/hm";
        home.stateVersion = "25.11";
        programs.f4 = {
          enable = true;
          # The module test must not build f4; the package is only placed
          # on home.packages.
          package = pkgs.hello;
          settings = sampleIni;
          keymap.Common = {
            CtrlAltO = "CtrlO";
            Alt1 = "F1";
          };
          hotkeys.Common.AltIns = "None";
          highlight.Highlight_100 = {
            Name = "Archives";
            Mask = "*.zip";
          };
        };
      }
    ];
  };

  # Touching every option and the activation entry forces them through the
  # module system's type checks and merges.
  evaluated = builtins.toJSON {
    inherit (hm.config.programs.f4) settings keymap hotkeys highlight;
    activation = hm.config.home.activation."f4-profile".data;
  };
in
pkgs.runCommand "f4-home-manager-module-check"
  {
    inherit evaluated;
  }
  ''
    set -euo pipefail
    upsert=${ini.upsert}/bin/f4-ini-upsert
    work=$(mktemp -d)
    cd "$work"

    # 1. ini.render golden: bools as 1/0, ints decimal, strings verbatim,
    # keys and sections sorted, blank line between sections.
    cat > render.expected <<'EOF'
    [Interface]
    ColorStyle = Radiola
    Num = 3
    ShowHidden = 0

    [Panel]
    Color = foreground:#FF00FF | background:#000080
    Mask = *.zip, *.7z
    EOF
    diff -u render.expected ${pkgs.writeText "render.ini" (ini.render sampleIni)}

    # 2a. merge: replace in place, append inside the section, new section at
    # the end - and preserve comments, unmanaged keys and blank lines.
    cat > target <<'EOF'
    ; keep me
    [Interface]
    ColorStyle = Radiola
    Language = ru

    [Panel]
    ShowHiddenFiles = 1
    Extra = stays
    EOF
    cat > desired <<'EOF'
    [Interface]
    ColorStyle = Dark
    Num = 3

    [New]
    A = 1
    B = two
    EOF
    cat > merged.expected <<'EOF'
    ; keep me
    [Interface]
    ColorStyle = Dark
    Language = ru
    Num = 3

    [Panel]
    ShowHiddenFiles = 1
    Extra = stays

    [New]
    A = 1
    B = two
    EOF
    "$upsert" target desired
    diff -u merged.expected target

    # 2b. idempotence: the same merge again changes nothing.
    cp target merged.once
    "$upsert" target desired
    cmp merged.once target

    # 2c. a missing TARGET is created from DESIRED.
    "$upsert" subdir/created desired
    diff -u desired subdir/created

    # 2d. duplicate keys are all replaced (f4 keeps the last one it reads)
    # and values holding "=" or "#" are left alone.
    cat > dup.target <<'EOF'
    [S]
    k = old
    k = older
    m = a=b#c
    EOF
    cat > dup.desired <<'EOF'
    [S]
    k = new
    EOF
    cat > dup.expected <<'EOF'
    [S]
    k = new
    k = new
    m = a=b#c
    EOF
    "$upsert" dup.target dup.desired
    diff -u dup.expected dup.target

    # 2e. a key before any section is refused.
    printf 'orphan = 1\n' > bad.desired
    rc=0
    "$upsert" bad.target bad.desired || rc=$?
    if [ "$rc" -ne 2 ]; then
      echo "expected exit 2 for a key outside any section, got $rc" >&2
      exit 1
    fi

    # 3. the home-manager evaluation above (the builder's environment
    # carries it) wired the activation entry at the platform's f4 profile.
    printf '%s' "$evaluated" | grep -Eq '(/\.config|Application Support)/f4/settings\.ini'

    touch "$out"
  ''
