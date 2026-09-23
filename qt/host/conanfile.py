from conan import ConanFile
from conan.errors import ConanInvalidConfiguration
from conan.tools.cmake import CMakeDeps, CMakeToolchain
from conan.tools.files import copy, save
from conan.tools.scm import Version
import os
import textwrap


class F4QtHostConan(ConanFile):
    settings = "os", "compiler", "build_type", "arch"
    options = {
        "with_video_thumbnails": [True, False],
        "with_ffmpeg_backend": [True, False],
    }
    default_options = {
        "with_video_thumbnails": False,
        "with_ffmpeg_backend": False,
        "qt/*:shared": True,
        "qt/*:qtdeclarative": True,
        "qt/*:qtsvg": True,
        "qt/*:qtmultimedia": True,
        "qt/*:qtshadertools": True,
        # The portable host excludes Qt's TLS plugin and does not make
        # network requests.  Disable OpenSSL so the static Windows package
        # stays self-contained without pulling a large crypto toolchain.
        "qt/*:openssl": False,
        "qt/*:with_libjpeg": "libjpeg-turbo",
        "qt/*:with_pq": False,
        "qt/*:with_odbc": False,
        "msgpack-cxx/*:use_boost": False,
        # HarfBuzz does not need GLib on Windows; disabling this integration
        # avoids the legacy gettext/GLib toolchain that is not MSVC portable.
        "harfbuzz/*:with_glib": False,
        "libtiff/*:jpeg": "libjpeg-turbo",
        "libraw/*:shared": True,
        "libraw/*:with_jpeg": "libjpeg-turbo",
        "libwebp/*:shared": False,
        "libjpeg-turbo/*:shared": False,
        "jasper/*:with_libjpeg": "libjpeg-turbo",
        "ffmpeg/*:shared": False,
        "ffmpeg/*:with_ssl": False,
        "ffmpeg/*:with_programs": False,
        # Qt only consumes decoder libraries. Avoid pulling optional encoder
        # stacks (and their GPL/size-heavy transitive graph) into the host.
        "ffmpeg/*:with_libx264": False,
        "ffmpeg/*:with_libx265": False,
        "ffmpeg/*:with_libfdk_aac": False,
        "ffmpeg/*:with_libsvtav1": False,
        "ffmpeg/*:with_libaom": False,
        # Keep the codec set broad without requiring a separate dav1d source
        # download; FFmpeg still retains its native AV1 decoder.
        "ffmpeg/*:with_libdav1d": False,
        "ffmpeg/*:with_openh264": False,
        "ffmpeg/*:with_libwebp": False,
    }

    def configure(self):
        video_thumbnails = bool(self.options.with_video_thumbnails)
        if not video_thumbnails:
            self.options.with_ffmpeg_backend = False
        self.options["qt/*"].qtmultimedia = video_thumbnails

    def requirements(self):
        self.requires("qt/6.11.1")
        if (self.options.with_video_thumbnails and
                self.options.with_ffmpeg_backend):
            # Keep FFmpeg in the same static graph as Qt Multimedia when the
            # FFmpeg backend is selected. Qt's backend supplies the built-in
            # H.264 and AV1 decoders used by video thumbnails; the optional
            # dav1d/openh264 accelerators remain disabled to avoid a second
            # codec toolchain.
            self.requires("ffmpeg/7.1.5")
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
        # ZoinGallery uses the current WebP API directly.  libtiff still
        # declares its older compatible WebP requirement transitively; make
        # the intended graph override explicit so static and shared builds
        # resolve the same ABI instead of failing on a version conflict.
        self.requires("libwebp/1.6.0", override=True)
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

            # Keep the native executable target list for the pre-project
            # include below. It must not be added to CMakeDeps' build-module
            # list: that list is also propagated into link metadata by the
            # Conan Qt facade, where a .cmake path becomes a linker input.
            native_qt_prefix = native_qt.package_folder.replace("\\", "/")
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
            native_tool_suffix = ".exe" if operating_system == "Windows" else ""
            for tool_name in native_tool_names:
                native_tools_body.extend(
                    [
                        f'  set(_f4_qt_native_tool "${{_f4_qt_native_bin}}/{tool_name}{native_tool_suffix}")',
                        f'  if(NOT TARGET Qt6::{tool_name})',
                        f'    add_executable(Qt6::{tool_name} IMPORTED GLOBAL)',
                        f'    set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION "${{_f4_qt_native_tool}}")',
                        f'    set_property(TARGET Qt6::{tool_name} PROPERTY IMPORTED_LOCATION_{build_type.upper()} "${{_f4_qt_native_tool}}")',
                        "  endif()",
                    ]
                )
            # QtDeclarative's in-tree cross build tests Qt::qsb, while the
            # Conan executable export is namespaced as Qt6::qsb. Mirror the
            # versionless tool target provided by Qt's native Tools config.
            native_tools_body.extend(
                [
                    '  if(NOT TARGET Qt::qsb AND TARGET Qt6::qsb)',
                    '    add_executable(Qt::qsb IMPORTED GLOBAL)',
                    '    get_target_property(_f4_qt_native_qsb Qt6::qsb IMPORTED_LOCATION)',
                    '    set_property(TARGET Qt::qsb PROPERTY IMPORTED_LOCATION "${_f4_qt_native_qsb}")',
                    '  endif()',
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
