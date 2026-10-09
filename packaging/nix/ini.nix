{ pkgs }:

# The ini vocabulary shared by f4's Home Manager module and its check: the
# value type f4's ini files accept, the renderer that spells values the way
# f4 writes them, and the merge tool that writes rendered keys into f4's
# live profile files.
let
  inherit (pkgs) lib;
in
rec {
  # A single ini value as f4's reader parses it: one line, "=" and the
  # section header being the only structure. Booleans spell the way the
  # files do, "1" and "0"; integers are written decimal; strings are
  # written verbatim and may not span lines.
  valueType = lib.types.oneOf [
    lib.types.bool
    lib.types.int
    (lib.types.strMatching "[^\r\n]*")
  ];

  # section name -> key -> value
  type = lib.types.attrsOf (lib.types.attrsOf valueType);

  renderValue = value:
    if builtins.isBool value then
      (if value then "1" else "0")
    else
      toString value;

  render = lib.generators.toINI {
    mkKeyValue = key: value: "${key} = ${renderValue value}";
  };

  upsert = pkgs.writeShellApplication {
    name = "f4-ini-upsert";
    runtimeInputs = [ pkgs.gawk ];
    text = builtins.readFile ./f4-ini-upsert.sh;
  };
}
