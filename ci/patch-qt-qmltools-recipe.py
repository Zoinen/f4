#!/usr/bin/env python3
"""Keep only Qt6::qmldom discoverable in Conan's native Qt cross package."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '        extension = ""\n'
_QML_TOOLS_CONFIG = r'''        # QtDeclarative's ARM64 cross-build needs the native qmldom executable.
        # Conan Center intentionally strips Qt6QmlToolsConfig.cmake because it
        # otherwise exposes every QML build tool to consumers.  Recreate a
        # deliberately minimal config after that cleanup: importing the full
        # generated export makes CMake try to define target-build executables
        # a second time in the cross build.
        qml_tools_dir = os.path.join(self.package_folder, "lib", "cmake", "Qt6QmlTools")
        os.makedirs(qml_tools_dir, exist_ok=True)
        save(self, os.path.join(qml_tools_dir, "Qt6QmlToolsConfig.cmake"), textwrap.dedent("""
            set(Qt6QmlTools_FOUND TRUE)
            if(NOT TARGET Qt6::qmldom)
              get_filename_component(_qt_qml_tools_prefix "${CMAKE_CURRENT_LIST_DIR}/../../../" ABSOLUTE)
              add_executable(Qt6::qmldom IMPORTED GLOBAL)
              set_target_properties(Qt6::qmldom PROPERTIES
                IMPORTED_LOCATION "${_qt_qml_tools_prefix}/bin/qmldom${CMAKE_EXECUTABLE_SUFFIX}")
            endif()
            set(Qt6QmlTools_TARGETS "Qt6::qmldom")
            """))

        extension = ""
'''


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    if text.count(_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: package() extension anchor is absent or ambiguous")
    if "def package(self):" not in text or "Qt6HostInfoConfig.cmake" not in text:
        raise SystemExit("unexpected Qt recipe: required Conan Center package cleanup is absent")
    if "Qt6QmlToolsConfig.cmake" in text:
        raise SystemExit("unexpected Qt recipe: Qt6QmlTools config is already customized")

    args.recipe.write_text(text.replace(_ANCHOR, _QML_TOOLS_CONFIG), encoding="utf-8")


if __name__ == "__main__":
    main()
