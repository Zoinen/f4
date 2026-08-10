from conan import ConanFile
from conan.errors import ConanInvalidConfiguration
from conan.tools.cmake import CMakeDeps, CMakeToolchain
from conan.tools.files import copy
from conan.tools.scm import Version
import os


class F4QtHostConan(ConanFile):
    settings = "os", "compiler", "build_type", "arch"
    options = {"with_zoingallery": [True, False]}
    default_options = {
        "with_zoingallery": False,
        # ZoinGallery deliberately builds its standalone application by
        # default. The f4 sidecar consumes only Core + the QML module.
        "zoingallery/*:build_standalone": False,
        "qt/*:shared": True,
        "qt/*:qtdeclarative": True,
        "qt/*:qtsvg": True,
        "qt/*:qtshadertools": True,
        "qt/*:with_pq": False,
        "qt/*:with_odbc": False,
        "msgpack-cxx/*:use_boost": False,
    }

    def requirements(self):
        self.requires("qt/6.11.1")
        self.requires("msgpack-cxx/7.0.0")
        if self.options.with_zoingallery:
            self.requires("zoingallery/0.1.0")

    def validate(self):
        if str(self.settings.os) != "Macos":
            return

        deployment_target = self.settings.get_safe("os.version")
        if deployment_target is None or Version(str(deployment_target)) != Version("13.0"):
            raise ConanInvalidConfiguration(
                "f4-qt-host requires Macos os.version=13.0 so Qt, codecs, "
                "ZoinGallery, and the host share one deployment target; "
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
        toolchain.variables["F4_WITH_ZOINGALLERY"] = bool(self.options.with_zoingallery)
        if operating_system == "Macos":
            toolchain.variables["CMAKE_OSX_DEPLOYMENT_TARGET"] = str(
                self.settings.os.version)

        if self.options.with_zoingallery:
            gallery_dependency = next(
                (dependency for dependency in self.dependencies.values()
                 if dependency.ref is not None
                 and dependency.ref.name == "zoingallery"),
                None,
            )
            if gallery_dependency is None:
                raise ConanInvalidConfiguration(
                    "with_zoingallery requires the zoingallery dependency")

            candidate_roots = [gallery_dependency.package_folder]
            for attribute in ("source_folder", "build_folder"):
                folder = getattr(gallery_dependency, attribute, None)
                if folder:
                    candidate_roots.append(folder)

            config_candidates = []
            for root in candidate_roots:
                if not root:
                    continue
                config_candidates.extend((
                    os.path.join(root, "lib", "cmake", "ZoinGallery"),
                    os.path.join(root, "build", "cmake", "ZoinGallery"),
                    os.path.join(root, "cmake", "ZoinGallery"),
                ))

            gallery_config_dir = next(
                (candidate for candidate in dict.fromkeys(config_candidates)
                 if os.path.isfile(os.path.join(
                     candidate, "ZoinGalleryConfig.cmake"))),
                None,
            )
            if gallery_config_dir is None:
                raise ConanInvalidConfiguration(
                    "zoingallery does not expose ZoinGalleryConfig.cmake; "
                    "checked: " + ", ".join(config_candidates))
            toolchain.variables["ZoinGallery_DIR"] = gallery_config_dir

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
                elif dependency_name == "zoingallery":
                    for resdir in dep.cpp_info.resdirs:
                        module_candidates = (
                            os.path.join(resdir, "ZoinGallery"),
                            resdir,
                        )
                        module_root = next(
                            (candidate for candidate in module_candidates
                             if os.path.isfile(os.path.join(candidate, "qmldir"))),
                            None,
                        )
                        if module_root:
                            copy(self, "*", src=module_root,
                                 dst=os.path.join("qml", "ZoinGallery"),
                                 keep_path=True)
                            break
