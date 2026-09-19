from conan import ConanFile
from conan.errors import ConanInvalidConfiguration
from conan.tools.cmake import CMakeDeps, CMakeToolchain
from conan.tools.files import copy
from conan.tools.scm import Version
import os


class F4QtHostConan(ConanFile):
    settings = "os", "compiler", "build_type", "arch"
    default_options = {
        "qt/*:shared": True,
        "qt/*:qtdeclarative": True,
        "qt/*:qtsvg": True,
        "qt/*:qtmultimedia": True,
        "qt/*:qtshadertools": True,
        "qt/*:with_pq": False,
        "qt/*:with_odbc": False,
        "msgpack-cxx/*:use_boost": False,
        "libtiff/*:jpeg": "libjpeg-turbo",
        "libraw/*:shared": True,
        "libraw/*:with_jpeg": "libjpeg-turbo",
        "libwebp/*:shared": False,
        "libjpeg-turbo/*:shared": False,
        "jasper/*:with_libjpeg": "libjpeg-turbo",
        "ffmpeg/*:shared": False,
        "ffmpeg/*:with_programs": False,
        # Qt only consumes decoder libraries. Avoid pulling optional encoder
        # stacks (and their GPL/size-heavy transitive graph) into the host.
        "ffmpeg/*:with_libx264": False,
        "ffmpeg/*:with_libx265": False,
        "ffmpeg/*:with_libfdk_aac": False,
        "ffmpeg/*:with_libsvtav1": False,
        "ffmpeg/*:with_libaom": False,
        "ffmpeg/*:with_libwebp": False,
    }

    def requirements(self):
        self.requires("qt/6.11.1")
        # Qt Multimedia's desktop backend is FFmpeg. Keep the codec runtime
        # in the same Conan graph so static portable builds link it instead of
        # depending on a machine-local ffmpeg executable or DLL set.
        self.requires("ffmpeg/7.1.5")
        self.requires("msgpack-cxx/7.0.0")
        # ZoinGallery is built from the pinned Git submodule. Keep its native
        # dependencies in this single Conan graph so the host and module share
        # one Qt runtime and one deployment ABI.
        self.requires("libtiff/4.7.0")
        self.requires("libraw/0.21.3")
        self.requires("libpng/1.6.45")
        self.requires("libwebp/1.6.0", override=True)
        self.requires("libheif/1.20.1")
        self.requires("libjpeg-turbo/3.0.2", override=True)
        self.requires("jasper/4.2.0", override=True)

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
