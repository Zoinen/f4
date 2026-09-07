package = "vtui"
version = "scm-1"
source = {
   url = "git+https://github.com/unxed/vtui.git"
}
description = {
   summary = "Declarative, stateful Desktop-class TUI and GUI framework for Lua",
   detailed = "Lua bindings and immediate-mode facade for the vtui framework.",
   homepage = "https://github.com/unxed/vtui",
   license = "BSD-3-Clause"
}
dependencies = {
   "lua >= 5.1"
}
build = {
   type = "builtin",
   modules = {
      vtui = "vtui.lua"
   }
}
