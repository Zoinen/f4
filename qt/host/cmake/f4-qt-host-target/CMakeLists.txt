set(F4_QT_HOST_ROOT "${CMAKE_CURRENT_LIST_DIR}/../..")

if(WIN32)
  set(F4_WINDOWS_ICON_FILE
    "${F4_QT_HOST_ROOT}/../../assets/icon/generated/f4.ico")
  if(NOT EXISTS "${F4_WINDOWS_ICON_FILE}")
    message(FATAL_ERROR "The generated Windows application icon is missing: ${F4_WINDOWS_ICON_FILE}")
  endif()
  configure_file(
    "${F4_QT_HOST_ROOT}/packaging/f4-qt-host.rc.in"
    "${CMAKE_CURRENT_BINARY_DIR}/f4-qt-host.rc"
    @ONLY)
  list(APPEND F4_QT_HOST_SOURCES
    "${CMAKE_CURRENT_BINARY_DIR}/f4-qt-host.rc")
endif()

set(F4_QT_HOST_QML_FILES
  "${F4_QT_HOST_ROOT}/qml/main.qml"
  "${F4_QT_HOST_ROOT}/qml/PanelSplitter.qml"
  "${F4_QT_HOST_ROOT}/qml/GalleryPanelHost.qml"
  "${F4_QT_HOST_ROOT}/qml/GalleryViewerHost.qml"
)

# Keep the historical resource URLs even though this deliberately clean
# target directory sits below the real QML source tree.
set_source_files_properties(
  "${F4_QT_HOST_ROOT}/qml/main.qml"
  PROPERTIES QT_RESOURCE_ALIAS "qml/main.qml")
set_source_files_properties(
  "${F4_QT_HOST_ROOT}/qml/PanelSplitter.qml"
  PROPERTIES QT_RESOURCE_ALIAS "qml/PanelSplitter.qml")
set_source_files_properties(
  "${F4_QT_HOST_ROOT}/qml/GalleryPanelHost.qml"
  PROPERTIES QT_RESOURCE_ALIAS "qml/GalleryPanelHost.qml")
set_source_files_properties(
  "${F4_QT_HOST_ROOT}/qml/GalleryViewerHost.qml"
  PROPERTIES QT_RESOURCE_ALIAS "qml/GalleryViewerHost.qml")

add_executable(f4-qt-host ${F4_QT_HOST_SOURCES})

if(F4_QT_STATIC AND MSVC AND CMAKE_GENERATOR STREQUAL "Ninja")
  # Ninja puts the final object and the complete static dependency closure on
  # one response-file line. link.exe truncates that line around 16 KiB, so
  # split CMake's multi-space argument separators just before linking.
  add_custom_command(TARGET f4-qt-host PRE_LINK
    COMMAND "${CMAKE_COMMAND}"
      "-DRSP_FILE=${CMAKE_CURRENT_BINARY_DIR}/CMakeFiles/f4-qt-host.rsp"
      -P "${F4_QT_HOST_ROOT}/cmake/SplitMsvcLinkResponse.cmake"
    VERBATIM)
endif()

# Qt's metatype and QML registration rules must be created in the same CMake
# directory as the executable target. Keeping both calls here also retains the
# narrow qmlimportscanner scope used by the static production build.
qt6_add_qml_module(f4-qt-host
  URI F4QtHost
  VERSION 1.0
  RESOURCE_PREFIX /
  SOURCES
    "${F4_QT_HOST_ROOT}/src/VtuiGridItem.h"
    "${F4_QT_HOST_ROOT}/src/VtuiGridItem.cpp"
  QML_FILES ${F4_QT_HOST_QML_FILES}
)

qt6_add_resources(f4-qt-host f4_lucide_icons
  PREFIX "/F4QtHost/icons/lucide"
  BASE "${F4_QT_HOST_ROOT}/icons/lucide"
  FILES ${F4_LUCIDE_ICONS}
)

qt6_add_resources(f4-qt-host f4_gallery_lucide_icons
  PREFIX "/F4QtHost/icons/lucide-gallery"
  BASE "${F4_GALLERY_LUCIDE_DIR}"
  FILES ${F4_GALLERY_LUCIDE_ICONS}
)

qt6_add_resources(f4-qt-host f4_app_icon
  PREFIX "/F4QtHost/icons/app"
  BASE "${F4_QT_HOST_ROOT}/../../assets/icon"
  FILES "${F4_QT_HOST_ROOT}/../../assets/icon/f4.svg"
)

target_include_directories(f4-qt-host PRIVATE "${F4_QT_HOST_ROOT}/src")

target_compile_definitions(f4-qt-host PRIVATE
  $<$<BOOL:${USE_QWK}>:__USE_QWK>
)

set_target_properties(f4-qt-host PROPERTIES
  RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin/$<CONFIG>"
)

if(APPLE)
  target_link_libraries(f4-qt-host PRIVATE "-framework Carbon")
  # The host binary is also copied into a small application bundle below so
  # LaunchServices can own the adaptive Icon Composer icon. Keep the bare
  # binary working as a compatibility fallback.
  set_target_properties(f4-qt-host PROPERTIES
    INSTALL_RPATH "@loader_path/../lib;@loader_path/../../../../lib"
  )

  set(F4_MACOS_ICON_DIR
    "${F4_QT_HOST_ROOT}/../../assets/icon/AppIcon.icon")
  set(F4_MACOS_ICON_FILES
    "${F4_MACOS_ICON_DIR}/icon.json"
    "${F4_MACOS_ICON_DIR}/Assets/f4-shell.svg"
    "${F4_MACOS_ICON_DIR}/Assets/f4-content.svg"
  )
  set(F4_MACOS_INFO_PLIST
    "${CMAKE_BINARY_DIR}/generated/f4-qt-host-Info.plist")
  configure_file("${F4_QT_HOST_ROOT}/packaging/f4-qt-host-Info.plist.in"
    "${F4_MACOS_INFO_PLIST}" @ONLY)

  # A source icon change must relink the host so this post-build packaging
  # step runs again even though actool's inputs are not C++ sources.
  set_property(SOURCE "${F4_QT_HOST_ROOT}/src/main.cpp" APPEND PROPERTY
    OBJECT_DEPENDS "${F4_MACOS_ICON_FILES}")

  set(F4_MACOS_APP_DIR
    "$<TARGET_FILE_DIR:f4-qt-host>/f4-qt-host.app")
  add_custom_command(TARGET f4-qt-host POST_BUILD
    COMMAND "${CMAKE_COMMAND}" -E make_directory
      "${F4_MACOS_APP_DIR}/Contents/MacOS"
      "${F4_MACOS_APP_DIR}/Contents/Resources"
    COMMAND xcrun actool "${F4_MACOS_ICON_DIR}"
      --compile "${F4_MACOS_APP_DIR}/Contents/Resources"
      --output-format human-readable-text
      --notices
      --warnings
      --output-partial-info-plist
        "${F4_MACOS_APP_DIR}/Contents/Resources/AppIcon-Partial.plist"
      --app-icon AppIcon
      --include-all-app-icons
      --enable-on-demand-resources NO
      --development-region en
      --target-device mac
      --minimum-deployment-target "${CMAKE_OSX_DEPLOYMENT_TARGET}"
      --platform macosx
    COMMAND "${CMAKE_COMMAND}" -E copy_if_different
      "${F4_MACOS_INFO_PLIST}"
      "${F4_MACOS_APP_DIR}/Contents/Info.plist"
    COMMAND "${CMAKE_COMMAND}" -E copy_if_different
      "$<TARGET_FILE:f4-qt-host>"
      "${F4_MACOS_APP_DIR}/Contents/MacOS/f4-qt-host"
    COMMENT "Building adaptive macOS f4-qt-host.app icon bundle"
    VERBATIM
  )
elseif(UNIX)
  set_target_properties(f4-qt-host PROPERTIES
    INSTALL_RPATH "$ORIGIN/../lib"
  )
endif()

# Prefer the in-tree ZoinGallery module over any older copy staged by Conan,
# then fall back to Conan's Qt QML import tree. The order also controls what
# qmlimportscanner embeds into static portable hosts.
set_property(TARGET f4-qt-host APPEND PROPERTY QT_QML_IMPORT_PATH
  "${ZoinGallery_QML_IMPORT_PATH}"
  "${CMAKE_BINARY_DIR}/qml"
)

target_link_libraries(f4-qt-host PRIVATE
  Qt6::Core
  Qt6::Gui
  Qt6::Qml
  Qt6::Quick
  Qt6::QuickControls2
  Qt6::Network
  Qt6::Svg
)
if(WIN32 AND F4_QT_STATIC)
  target_link_libraries(f4-qt-host PRIVATE Rpcrt4 Usp10)
endif()
if(USE_QWK)
  target_link_libraries(f4-qt-host PRIVATE QWindowKit::Quick)
endif()

target_link_libraries(f4-qt-host PRIVATE
  ZoinGallery::Core
  ZoinGallery::Qml
)
if(TARGET msgpack-cxx)
  target_link_libraries(f4-qt-host PRIVATE msgpack-cxx)
elseif(TARGET msgpack-cxx::msgpack-cxx)
  target_link_libraries(f4-qt-host PRIVATE msgpack-cxx::msgpack-cxx)
else()
  message(FATAL_ERROR "msgpack-cxx CMake target was not generated by Conan")
endif()

if(F4_QT_STATIC)
  # QtQuick.Controls lists every installed style as an optional import. The
  # generic scanner therefore links all of them into a static executable even
  # though main.cpp selects Basic before loading QML. Register the production
  # module set explicitly: this avoids unused style resources/initializers and
  # keeps the MSVC response file below its practical per-line limit.
  set_property(TARGET f4-qt-host PROPERTY QT_QML_MODULE_NO_IMPORT_SCAN TRUE)
  set(F4_REQUIRED_STATIC_QML_PLUGINS
    Qt6::qmlplugin
    Qt6::modelsplugin
    Qt6::workerscriptplugin
    Qt6::qtquick2plugin
    Qt6::quickwindow
    Qt6::effectsplugin
    Qt6::qmlshapesplugin
    Qt6::qquicklayoutsplugin
    Qt6::qtquicktemplates2plugin
    Qt6::qtquickcontrols2implplugin
    Qt6::qtquickcontrols2basicstyleimplplugin
    Qt6::qtquickcontrols2basicstyleplugin
    Qt6::qtquickcontrols2plugin
    ZoinGallery::ZoinGalleryQmlplugin
  )

  # Conan Center removes the native Qt *Config.cmake files after packaging
  # and recreates only the components declared by its recipe. Qt's QML plugin
  # libraries are packaged, along with their official Targets.cmake metadata,
  # but are not Conan components. Import those preserved targets explicitly so
  # a static Conan Qt behaves like an ordinary static Qt installation.
  set(_f4_conan_qt_package_folder "")
  foreach(_f4_config RELEASE RELWITHDEBINFO MINSIZEREL DEBUG)
    if(DEFINED qt_PACKAGE_FOLDER_${_f4_config}
       AND EXISTS "${qt_PACKAGE_FOLDER_${_f4_config}}/lib/cmake/Qt6Qml/QmlPlugins")
      set(_f4_conan_qt_package_folder
        "${qt_PACKAGE_FOLDER_${_f4_config}}")
      break()
    endif()
  endforeach()
  if(_f4_conan_qt_package_folder)
    set(_f4_qml_plugin_cmake_dir
      "${_f4_conan_qt_package_folder}/lib/cmake/Qt6Qml/QmlPlugins")

    # Qt's native Qt6Targets.cmake normally provides these interface-only
    # implementation-detail targets. Conan's generated Qt6 targets omit them,
    # while the preserved static QML plugin exports still reference them.
    # Recreate the small dependency chain from QtBaseGlobalTargets.cmake before
    # loading any plugin export so its generated dependency checks can pass.
    if(NOT TARGET Qt6::PlatformCommonInternal)
      add_library(Qt6::PlatformCommonInternal INTERFACE IMPORTED GLOBAL)
      set_target_properties(Qt6::PlatformCommonInternal PROPERTIES
        INTERFACE_LINK_LIBRARIES "Qt6::Platform")
    endif()
    foreach(_f4_platform_target IN ITEMS
        PlatformModuleInternal PlatformPluginInternal)
      if(NOT TARGET Qt6::${_f4_platform_target})
        add_library(Qt6::${_f4_platform_target} INTERFACE IMPORTED GLOBAL)
        set_target_properties(Qt6::${_f4_platform_target} PROPERTIES
          INTERFACE_LINK_LIBRARIES "Qt6::PlatformCommonInternal")
      endif()
    endforeach()

    function(_f4_import_conan_qt_qml_backing_module module_name)
      if(TARGET Qt6::${module_name})
        return()
      endif()
      set(_f4_archive_var "_f4_${module_name}_archive")
      find_library(${_f4_archive_var}
        NAMES Qt6${module_name}
        PATHS "${_f4_conan_qt_package_folder}/lib"
        NO_DEFAULT_PATH REQUIRED)
      add_library(Qt6::${module_name} STATIC IMPORTED GLOBAL)
      set_target_properties(Qt6::${module_name} PROPERTIES
        IMPORTED_LOCATION "${${_f4_archive_var}}"
        INTERFACE_LINK_LIBRARIES "${ARGN}")
      unset(${_f4_archive_var} CACHE)
    endfunction()

    _f4_import_conan_qt_qml_backing_module(QmlMeta
      Qt6::Core Qt6::Qml Qt6::QmlModels Qt6::QmlWorkerScript)
    _f4_import_conan_qt_qml_backing_module(QuickControls2Impl
      Qt6::Core Qt6::Gui Qt6::Quick Qt6::Qml Qt6::QuickTemplates2)
    _f4_import_conan_qt_qml_backing_module(QuickControls2BasicStyleImpl
      Qt6::Core Qt6::Gui Qt6::Qml Qt6::QuickControls2Impl Qt6::Quick
      Qt6::QuickTemplates2)
    _f4_import_conan_qt_qml_backing_module(QuickControls2Basic
      Qt6::Core Qt6::Gui Qt6::Qml Qt6::QuickControls2BasicStyleImpl
      Qt6::QuickControls2 Qt6::Quick Qt6::QuickTemplates2)
    _f4_import_conan_qt_qml_backing_module(QuickEffects
      Qt6::Core Qt6::Gui Qt6::Qml Qt6::Quick)
    _f4_import_conan_qt_qml_backing_module(QuickLayouts
      Qt6::Core Qt6::Qml Qt6::Gui Qt6::Quick)

    if(NOT TARGET Qt6::QOffscreenIntegrationPlugin)
      find_library(_f4_qoffscreen_archive
        NAMES qoffscreen
        PATHS "${_f4_conan_qt_package_folder}/plugins/platforms"
        NO_DEFAULT_PATH REQUIRED)
      add_library(Qt6::QOffscreenIntegrationPlugin STATIC IMPORTED GLOBAL)
      set_target_properties(Qt6::QOffscreenIntegrationPlugin PROPERTIES
        IMPORTED_LOCATION "${_f4_qoffscreen_archive}"
        INTERFACE_LINK_LIBRARIES "Qt6::Core;Qt6::Gui")
      unset(_f4_qoffscreen_archive CACHE)
    endif()

    # Static Qt intentionally gives qmlplugin a dependency on modelsplugin and
    # workerscriptplugin, while those plugins in turn depend on the QtQml
    # module represented by qmlplugin. CMake's generated export files support
    # such cross-export cycles when loaded by find_package(): they report a
    # package as not found instead of failing immediately, allowing the other
    # exports to be loaded. Reproduce that include context locally, then rely
    # on the explicit target validation below after every export exists.
    function(_f4_include_conan_qt_qml_plugin_export
        targets_file additional_info_file)
      set(CMAKE_FIND_PACKAGE_NAME F4ConanStaticQmlPluginExport)
      include("${targets_file}")
      if(EXISTS "${additional_info_file}")
        include("${additional_info_file}")
      endif()
    endfunction()

    foreach(plugin_target IN LISTS F4_REQUIRED_STATIC_QML_PLUGINS)
      if(NOT TARGET ${plugin_target} AND plugin_target MATCHES "^Qt6::(.+)$")
        set(_f4_plugin_name "${CMAKE_MATCH_1}")
        set(_f4_plugin_targets_file
          "${_f4_qml_plugin_cmake_dir}/Qt6${_f4_plugin_name}Targets.cmake")
        if(EXISTS "${_f4_plugin_targets_file}")
          set(_f4_plugin_info_file
            "${_f4_qml_plugin_cmake_dir}/Qt6${_f4_plugin_name}AdditionalTargetInfo.cmake")
          _f4_include_conan_qt_qml_plugin_export(
            "${_f4_plugin_targets_file}" "${_f4_plugin_info_file}")
        endif()
      endif()
    endforeach()
  endif()
  foreach(plugin_target IN LISTS F4_REQUIRED_STATIC_QML_PLUGINS)
    if(NOT TARGET ${plugin_target})
      message(FATAL_ERROR
        "Required static QML plugin target is missing: ${plugin_target}")
    endif()
  endforeach()
  set(F4_REQUIRED_STATIC_QT_PLUGINS
    Qt6::QOffscreenIntegrationPlugin
    Qt6::QGifPlugin
    Qt6::QIcoPlugin
    Qt6::QJpegPlugin
    Qt6::QSvgPlugin
  )
  if(WIN32)
    list(APPEND F4_REQUIRED_STATIC_QT_PLUGINS
      Qt6::QWindowsIntegrationPlugin)
  elseif(UNIX AND NOT APPLE)
    list(APPEND F4_REQUIRED_STATIC_QT_PLUGINS
      Qt6::QXcbIntegrationPlugin)
  endif()
  foreach(plugin_target IN LISTS F4_REQUIRED_STATIC_QT_PLUGINS)
    if(NOT TARGET ${plugin_target})
      message(FATAL_ERROR
        "Required static Qt plugin target is missing: ${plugin_target}")
    endif()
  endforeach()
  target_sources(f4-qt-host PRIVATE
    "${F4_QT_HOST_ROOT}/src/StaticQtPluginImports.cpp"
    "${F4_QT_HOST_ROOT}/src/StaticQmlPluginImports.cpp")
  target_link_libraries(f4-qt-host PRIVATE
    ${F4_REQUIRED_STATIC_QML_PLUGINS}
    ${F4_REQUIRED_STATIC_QT_PLUGINS}
    Qt6::QuickControls2Basic
    Qt6::QuickControls2BasicStyleImpl
    Qt6::QuickControls2Impl
    Qt6::QuickEffects
    Qt6::QuickLayouts)

  # Conan exposes Qt's Linux platform implementation as separate static
  # archives with a real cycle: qxcb -> Qt6XcbQpa -> Qt6Gui. GNU ld scans an
  # ordinary archive only once, so rescan this small platform group until all
  # of its private Qt symbols are resolved. Windows' linker does not need it.
  if(UNIX AND NOT APPLE)
    find_library(_f4_qxcb_archive NAMES qxcb
      PATHS "${_f4_conan_qt_package_folder}/plugins/platforms"
      NO_DEFAULT_PATH REQUIRED)
    find_library(_f4_xcb_qpa_archive NAMES Qt6XcbQpa
      PATHS "${_f4_conan_qt_package_folder}/lib"
      NO_DEFAULT_PATH REQUIRED)
    find_library(_f4_gui_archive NAMES Qt6Gui
      PATHS "${_f4_conan_qt_package_folder}/lib"
      NO_DEFAULT_PATH REQUIRED)
    target_link_libraries(f4-qt-host PRIVATE
      "$<LINK_GROUP:RESCAN,${_f4_qxcb_archive},${_f4_xcb_qpa_archive},${_f4_gui_archive}>")
    unset(_f4_qxcb_archive CACHE)
    unset(_f4_xcb_qpa_archive CACHE)
    unset(_f4_gui_archive CACHE)
  endif()

  # The production host never enables QML debugging, so exclude the ordinary
  # debugger/profiler transport plugins supplied by Qt::Qml as well.
  qt6_import_plugins(f4-qt-host EXCLUDE_BY_TYPE qmltooling)
endif()

qt6_finalize_target(f4-qt-host)
