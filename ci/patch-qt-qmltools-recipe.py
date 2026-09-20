#!/usr/bin/env python3
"""Expose native Qt QML tools needed by Conan's cross-build package."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '        extension = ""\n'
_VERSION_CONFIG = r'''        save(self, os.path.join(qml_tools_dir, "Qt6QmlToolsConfigVersion.cmake"), textwrap.dedent("""
            set(PACKAGE_VERSION "6.11.1")
            if(PACKAGE_FIND_VERSION VERSION_EQUAL PACKAGE_VERSION)
              set(PACKAGE_VERSION_COMPATIBLE TRUE)
              set(PACKAGE_VERSION_EXACT TRUE)
            elseif(PACKAGE_FIND_VERSION VERSION_LESS PACKAGE_VERSION)
              set(PACKAGE_VERSION_COMPATIBLE TRUE)
            endif()
            """))
'''
_CUSTOM_CONFIG_BEGIN = "        # QtDeclarative's ARM64 cross-build needs the native qmldom executable.\n"
_QML_TOOLS_CONFIG_MARKER = "add_executable(Qt6::${_qt_qml_tool} IMPORTED GLOBAL)"
_QML_TOOLS_CONFIG = r'''        # QtDeclarative's ARM64 cross-build needs the native qmldom executable.
        # Conan Center intentionally strips Qt6QmlToolsConfig.cmake because it
        # otherwise exposes every QML build tool to consumers. Recreate only
        # the native QML executables needed by the target cross-build after
        # that cleanup: importing the full generated export makes CMake try to
        # define target-build executables a second time in the cross build.
        qml_tools_dir = os.path.join(self.package_folder, "lib", "cmake", "Qt6QmlTools")
        os.makedirs(qml_tools_dir, exist_ok=True)
        save(self, os.path.join(qml_tools_dir, "Qt6QmlToolsConfig.cmake"), textwrap.dedent("""
            set(Qt6QmlTools_FOUND TRUE)
            get_filename_component(_qt_qml_tools_prefix "${CMAKE_CURRENT_LIST_DIR}/../../../" ABSOLUTE)
            set(_qt_qml_tools
              qmldom
              qmlcachegen
              qmlcontextpropertydump
              qmleasing
              qmlformat
              qmlimportscanner
              qmljsrootgen
              qmllint
              qml
              qmlaotstats
              qmlpreview
              qmlprofiler
              qmltc
              qmltestrunner
              qmltyperegistrar
            )
            set(Qt6QmlTools_TARGETS "")
            foreach(_qt_qml_tool IN LISTS _qt_qml_tools)
              if(NOT TARGET Qt6::${_qt_qml_tool})
                add_executable(Qt6::${_qt_qml_tool} IMPORTED GLOBAL)
                set_target_properties(Qt6::${_qt_qml_tool} PROPERTIES
                  IMPORTED_LOCATION "${_qt_qml_tools_prefix}/bin/${_qt_qml_tool}${CMAKE_EXECUTABLE_SUFFIX}")
              endif()
              list(APPEND Qt6QmlTools_TARGETS "Qt6::${_qt_qml_tool}")
            endforeach()
            """))
''' + _VERSION_CONFIG + r'''

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
        # Conan exports are reused by interrupted CI jobs.  Keep the patch
        # idempotent so a recipe copied from an already customized cache does
        # not make the ARM64 job fail before the actual build starts.
        required_markers = (
            "set(Qt6QmlTools_FOUND TRUE)",
            (
                "add_executable(Qt6::qmldom IMPORTED GLOBAL)",
                _QML_TOOLS_CONFIG_MARKER,
            ),
        )
        if not (
            all(marker in text for marker in required_markers[:1])
            and any(marker in text for marker in required_markers[1])
        ):
            raise SystemExit("unexpected Qt recipe: Qt6QmlTools config is already customized")
        if _VERSION_CONFIG in text and _QML_TOOLS_CONFIG_MARKER in text:
            return
        custom_start = text.find(_CUSTOM_CONFIG_BEGIN)
        custom_end = text.find(_ANCHOR, custom_start)
        if custom_start < 0 or custom_end < 0:
            raise SystemExit("unexpected Qt recipe: customized Qt6QmlTools block is malformed")
        upgraded = text[:custom_start] + _QML_TOOLS_CONFIG + text[custom_end + len(_ANCHOR):]
        args.recipe.write_text(upgraded, encoding="utf-8")
        return

    args.recipe.write_text(text.replace(_ANCHOR, _QML_TOOLS_CONFIG), encoding="utf-8")


if __name__ == "__main__":
    main()
