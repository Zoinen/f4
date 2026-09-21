from conan import ConanFile
from conan.errors import ConanInvalidConfiguration
from conan.tools.cmake import CMakeDeps, CMakeToolchain
from conan.tools.files import copy, load, save
from conan.tools.scm import Version
import os
import textwrap


class F4QtHostConan(ConanFile):
    settings = "os", "compiler", "build_type", "arch"
    default_options = {
        "qt/*:shared": True,
        "qt/*:qtdeclarative": True,
        "qt/*:qtsvg": True,
        "qt/*:qtshadertools": True,
        "qt/*:with_pq": False,
        "qt/*:with_odbc": False,
        "msgpack-cxx/*:use_boost": False,
        "libtiff/*:jpeg": "libjpeg-turbo",
        "libraw/*:shared": True,
        "libraw/*:with_jpeg": "libjpeg-turbo",
        "libwebp/*:shared": False,
        "jasper/*:with_libjpeg": "libjpeg-turbo",
    }

    def requirements(self):
        self.requires("qt/6.11.1")
        self.requires("msgpack-cxx/7.0.0")
        # ZoinGallery is built from the pinned Git submodule. Keep its native
        # dependencies in this single Conan graph so the host and module share
        # one Qt runtime and one deployment ABI.
        # harfbuzz/8.3.0 still pins freetype/2.13.2 while Qt exposes a
        # compatible version range that otherwise resolves to 2.13.3. Keep
        # the graph deterministic until Conan Center removes that mismatch.
        self.requires("freetype/2.13.2", override=True)
        self.requires("libtiff/4.7.0")
        self.requires("libraw/0.21.3")
        self.requires("libpng/1.6.45")
        self.requires("libwebp/1.6.0")
        self.requires("libheif/1.20.1")
        self.requires("libjpeg-turbo/3.0.2", override=True)
        self.requires("jasper/4.2.0", override=True)

    def build_requirements(self):
        # Windows ARM is cross-compiled by the x64 GitHub runner. Keep a
        # native Qt package in this consumer graph as well as the target Qt
        # requirement so CMake's AUTOGEN/QML tools can run during configure.
        # The Qt recipe already uses the same build-context package while it
        # builds the target Qt libraries; making it explicit here exposes its
        # package folder to the f4-qt and QWindowKit generators too.
        if str(self.settings.os) == "Windows" and str(self.settings.arch) == "armv8":
            self.tool_requires("qt/6.11.1")

    def validate(self):
        if str(self.settings.os) != "Macos":
            return

        deployment_target = self.settings.get_safe("os.version")
        if deployment_target is None or Version(str(deployment_target)) != Version("13.0"):
            raise ConanInvalidConfiguration(
                "f4-qt-host requires Macos os.version=13.0 so Qt, codecs, "
                "the bundled ZoinGallery module and host share one deployment target; "
                "pass '-s:h os.version=13.0' to conan install")

    def layout(self):
        self.cpp.build.libdirs = "lib"
        self.cpp.build.bindirs = "bin"

    def generate(self):
        build_type = str(self.settings.build_type)
        compiler = str(self.settings.compiler)
        operating_system = str(self.settings.os)

        dependencies = CMakeDeps(self)
        dependencies.generate()
        toolchain = CMakeToolchain(self)
        if operating_system == "Macos":
            toolchain.variables["CMAKE_OSX_DEPLOYMENT_TARGET"] = str(
                self.settings.os.version)

        # In a cross build Conan exposes Qt's native build-context package
        # through the transitive build requirements of the target Qt package.
        # Prefer its QML/shader tool configs explicitly: the target package
        # can contain same-named metadata whose qsb/qml executables have the
        # target architecture and cannot run on the build runner.
        visited_dependencies = set()

        def find_native_qt(dependencies):
            for dep in dependencies.values():
                dependency_key = (
                    str(dep.ref),
                    str(dep.package_folder),
                    str(dep.context),
                )
                if dependency_key in visited_dependencies:
                    continue
                visited_dependencies.add(dependency_key)

                if (
                    dep.ref is not None
                    and dep.ref.name == "qt"
                    and dep.is_build_context
                    and dep.package_folder
                ):
                    return dep

                nested = find_native_qt(dep.dependencies.build)
                if nested is not None:
                    return nested
                nested = find_native_qt(dep.dependencies.host)
                if nested is not None:
                    return nested
            return None

        native_qt = find_native_qt(self.dependencies.build)
        if native_qt is None:
            native_qt = find_native_qt(self.dependencies.host)
        if native_qt is not None:
            self.output.info(
                f"Using native Qt build-context tools from {native_qt.package_folder}"
            )
            native_qt_cmake_dir = os.path.join(native_qt.package_folder, "lib", "cmake")
            toolchain.cache_variables["Qt6QmlTools_DIR"] = os.path.join(
                native_qt_cmake_dir, "Qt6QmlTools"
            )
            toolchain.cache_variables["Qt6ShaderToolsTools_DIR"] = os.path.join(
                native_qt_cmake_dir, "Qt6ShaderToolsTools"
            )

            # CMakeDeps also emits Qt6::moc/rcc/uic and the other Qt host
            # tools from the target package's conan_qt_executables_variables
            # module. In a Windows ARM cross-build those executables have the
            # target architecture and cannot run on the x64 GitHub runner.
            # Append a build module after Conan's Qt modules so the imported
            # tool targets keep the target package for headers/libraries but
            # execute from the native Qt build-context package.
            native_qt_prefix = native_qt.package_folder.replace("\\", "/")
            native_tools_file = os.path.join(
                self.generators_folder, "f4-qt-native-tools.cmake"
            )
            native_tool_names = (
                "moc",
                "qlalr",
                "rcc",
                "tracegen",
                "cmake_automoc_parser",
                "qmake",
                "qtpaths",
                "syncqt",
                "tracepointgen",
                "qvkgen",
                "uic",
                "windeployqt",
                "wasmdeployqt",
                "qsb",
                "qmltyperegistrar",
                "qmlcachegen",
                "qmllint",
                "qmlimportscanner",
                "qmlformat",
                "qml",
                "qmlprofiler",
                "qmlpreview",
                "qmltc",
                "qmlaotstats",
            )
            native_tools_body = [
                f'set(_f4_qt_native_bin "{native_qt_prefix}/bin")',
            ]
            for tool_name in native_tool_names:
                native_tools_body.extend(
                    [
                        f'set(_f4_qt_native_tool "${{_f4_qt_native_bin}}/{tool_name}")',
                        f'if(NOT EXISTS "${{_f4_qt_native_tool}}" AND EXISTS "${{_f4_qt_native_bin}}/{tool_name}.exe")',
                        f'  set(_f4_qt_native_tool "${{_f4_qt_native_bin}}/{tool_name}.exe")',
                        "endif()",
                        f'if(TARGET Qt6::{tool_name} AND EXISTS "${{_f4_qt_native_tool}}")',
                        f'  set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION "${{_f4_qt_native_tool}}")',
                        f'  set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION_{build_type.upper()} "${{_f4_qt_native_tool}}")',
                        "endif()",
                    ]
                )
            native_tools_body.extend(
                [
                    "unset(_f4_qt_native_tool)",
                    "unset(_f4_qt_native_bin)",
                ]
            )
            save(self, native_tools_file, "\n".join(native_tools_body) + "\n")

            # CMakeDeps names this file Qt6-<configuration>-<arch>-data.cmake
            # and stores the build-module list there. Patch every generated
            # Qt data file so the same native-tool routing is used by both
            # QWindowKit's standalone configure and the main f4 configure.
            build_modules_variable = f"qt_BUILD_MODULES_PATHS_{build_type.upper()}"
            native_tools_path = native_tools_file.replace("\\", "/")
            for generated_name in os.listdir(self.generators_folder):
                generated_file = os.path.join(self.generators_folder, generated_name)
                if not os.path.isfile(generated_file) or not generated_name.endswith(
                    ".cmake"
                ):
                    continue
                generated_text = load(self, generated_file)
                if native_tools_path in generated_text:
                    continue

                generated_lines = generated_text.splitlines()
                start = next(
                    (
                        index
                        for index, line in enumerate(generated_lines)
                        if line.startswith(f"set({build_modules_variable} ")
                    ),
                    None,
                )
                if start is None:
                    continue

                end = next(
                    (
                        index
                        for index in range(start + 1, len(generated_lines))
                        if generated_lines[index].strip() == ")"
                    ),
                    None,
                )
                if end is None:
                    raise ConanInvalidConfiguration(
                        f"cannot extend {generated_name}: malformed Qt build-module list"
                    )
                generated_lines.insert(end, f'\t\t\t"{native_tools_path}"')
                save(self, generated_file, "\n".join(generated_lines) + "\n")

        # CMakeDeps puts the native Qt build-context prefixes before the
        # target prefixes in CMAKE_PREFIX_PATH, but Qt's own package config
        # can still select the target package's same-named tool metadata.
        # Resolve the first prefix containing each tool config before any
        # project() call. This keeps native qsb/qml tools on the build runner
        # while preserving the target package for libraries and headers.
        cross_tools_file = os.path.join(
            self.generators_folder, "f4-qt-cross-tools.cmake"
        )
        cross_tools_text = textwrap.dedent(
            """
            foreach(_f4_qt_prefix IN LISTS CMAKE_PREFIX_PATH)
              if(NOT Qt6QmlTools_DIR AND EXISTS "${_f4_qt_prefix}/Qt6QmlToolsConfig.cmake")
                set(Qt6QmlTools_DIR "${_f4_qt_prefix}" CACHE PATH "Native Qt QML tools" FORCE)
              endif()
              if(NOT Qt6ShaderToolsTools_DIR AND EXISTS "${_f4_qt_prefix}/Qt6ShaderToolsToolsConfig.cmake")
                set(Qt6ShaderToolsTools_DIR "${_f4_qt_prefix}" CACHE PATH "Native Qt shader tools" FORCE)
              endif()
            endforeach()
            """
        )
        if native_qt is not None:
            # Conan's Qt package build module declares Qt6::moc/rcc/uic and
            # the other executable targets with an ``if(NOT TARGET ...)``
            # guard. Declare those imported targets before Qt's package is
            # loaded so the target package cannot install ARM64 executables
            # which the x64 build runner cannot execute. The target package
            # still supplies all headers and libraries; only the host tools
            # come from the native build-context package.
            native_tools_body = [
                f'set(_f4_qt_native_prefix "{native_qt_prefix}")',
                '  set(_f4_qt_native_bin "${_f4_qt_native_prefix}/bin")',
            ]
            for tool_name in native_tool_names:
                native_tools_body.extend(
                    [
                        f'  set(_f4_qt_native_tool "${{_f4_qt_native_bin}}/{tool_name}")',
                        f'  if(NOT EXISTS "${{_f4_qt_native_tool}}" AND EXISTS "${{_f4_qt_native_bin}}/{tool_name}.exe")',
                        f'    set(_f4_qt_native_tool "${{_f4_qt_native_bin}}/{tool_name}.exe")',
                        "  endif()",
                        f'  if(NOT TARGET Qt6::{tool_name} AND EXISTS "${{_f4_qt_native_tool}}")',
                        f'    add_executable(Qt6::{tool_name} IMPORTED GLOBAL)',
                        f'    set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION "${{_f4_qt_native_tool}}")',
                        f'    set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION_{build_type.upper()} "${{_f4_qt_native_tool}}")',
                        "  endif()",
                    ]
                )
            native_tools_body.extend(
                [
                    "  unset(_f4_qt_native_tool)",
                    "  unset(_f4_qt_native_bin)",
                    "unset(_f4_qt_native_prefix)",
                ]
            )
            cross_tools_text += "\n" + "\n".join(native_tools_body) + "\n"
        save(
            self,
            cross_tools_file,
            cross_tools_text,
        )
        toolchain.cache_variables["CMAKE_PROJECT_INCLUDE_BEFORE"] = cross_tools_file

        toolchain.generate()

        for dep in self.dependencies.values():
            if compiler == "apple-clang":
                for libdir in dep.cpp_info.libdirs:
                    copy(self, "*.dylib", src=libdir, dst=os.path.join(self.cpp.build.libdirs, build_type), keep_path=False)
            elif compiler == "gcc":
                for libdir in dep.cpp_info.libdirs:
                    copy(self, "*.so", src=libdir, dst=os.path.join(self.cpp.build.libdirs, build_type), keep_path=False)
            elif compiler == "msvc":
                for bindir in dep.cpp_info.bindirs:
                    copy(self, "*.dll", src=bindir, dst=os.path.join(self.cpp.build.bindirs, build_type), keep_path=False)

            if dep.ref is not None:
                dependency_name = dep.ref.name
                package_folder = dep.package_folder
                if dependency_name == "qt":
                    plugin_source = os.path.join(package_folder, "plugins")
                    if os.path.isdir(plugin_source):
                        plugin_pattern = {
                            "apple-clang": "*.dylib",
                            "gcc": "*.so*",
                            "msvc": "*.dll",
                        }.get(compiler)
                        if plugin_pattern:
                            copy(self, plugin_pattern, src=plugin_source,
                                 dst="plugins", keep_path=True)

                    # Conan's Qt CMake metadata does not expose Qt's original
                    # QML install prefix. Stage the import tree on every
                    # platform so qmlimportscanner and the deploy helper can
                    # resolve the required Qt Quick modules without a
                    # development Qt installation.
                    qml_source = os.path.join(package_folder, "qml")
                    if os.path.isdir(qml_source):
                        copy(self, "*", src=qml_source, dst="qml", keep_path=True)
