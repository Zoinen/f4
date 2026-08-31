cmake_minimum_required(VERSION 3.23)

if(POLICY CMP0091)
  cmake_policy(SET CMP0091 NEW)
endif()

set(CMAKE_OSX_DEPLOYMENT_TARGET "13.0" CACHE STRING "Minimum macOS deployment target" FORCE)

project(F4QtHost LANGUAGES CXX)

if(APPLE)
  enable_language(OBJCXX)
endif()

option(USE_QWK "Use QWindowKit" ON)
option(F4_PORTABLE_STATIC
  "Build one self-contained Qt host (apart from system ABI libraries)" OFF)
include(CTest)

if(NOT EXISTS "${CMAKE_BINARY_DIR}/conan_toolchain.cmake")
  message(FATAL_ERROR "The file conan_toolchain.cmake does not exist. Run: conan install . --build=missing -s build_type=RelWithDebInfo -s compiler.cppstd=20 --output-folder=build")
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_AUTOMOC ON)

if(F4_PORTABLE_STATIC)
  # Qt discovers its JPEG dependency from inside a helper function. Conan's
  # generated FindJPEG module creates directory-scoped imported targets and
  # then guards itself against a second include, so ZoinGallery cannot see
  # JPEG::JPEG later in this sibling directory. Load both facades at the root
  # first so their static targets are visible to the complete build graph.
  find_package(JPEG REQUIRED)
  find_package(libjpeg-turbo REQUIRED)
endif()

find_package(Qt6 COMPONENTS Core Concurrent Gui Qml Quick QuickControls2 Network Svg REQUIRED)
find_package(msgpack-cxx REQUIRED)
if(USE_QWK)
  find_package(QWindowKit REQUIRED)
endif()

if(F4_PORTABLE_STATIC)
  # Qt's Conan facade omits Qt6::OpenGL from Qt6::Quick's static link
  # interface even though libQt6Quick.a contains the painter/FBO backend.
  # Append it transitively so every host and test target that consumes Quick
  # receives libQt6OpenGL in the correct archive order.
  find_package(Qt6 COMPONENTS OpenGL REQUIRED)
  set_property(TARGET Qt6::Quick APPEND PROPERTY
    INTERFACE_LINK_LIBRARIES Qt6::OpenGL)

  if(UNIX AND NOT APPLE)
    # The portable Linux host supports both native Wayland/EGL and the XCB
    # fallback. Conan packages the static Wayland module and its plugin
    # archives, but does not expose the platform plugins as Qt components.
    find_package(Qt6 COMPONENTS WaylandClient REQUIRED)
    find_package(wayland REQUIRED)
    find_package(xkbcommon REQUIRED)
  endif()

  # A native Qt installation exposes Qt6::Core as STATIC_LIBRARY, while the
  # Conan package exposes the same static archives through an INTERFACE_LIBRARY
  # facade. Reject a directly shared target here; the portable CI audits the
  # final executable's imports to prove that no Qt or compiler runtime library
  # escaped into the result.
  get_target_property(F4_QT_CORE_TYPE Qt6::Core TYPE)
  if(NOT F4_QT_CORE_TYPE STREQUAL "STATIC_LIBRARY" AND
     NOT F4_QT_CORE_TYPE STREQUAL "INTERFACE_LIBRARY")
    message(FATAL_ERROR
      "F4_PORTABLE_STATIC requires a static Qt package; Qt6::Core is ${F4_QT_CORE_TYPE}")
  endif()
  if(USE_QWK)
    get_target_property(F4_QWK_QUICK_TYPE QWindowKit::Quick TYPE)
    if(NOT F4_QWK_QUICK_TYPE STREQUAL "STATIC_LIBRARY" AND
       NOT F4_QWK_QUICK_TYPE STREQUAL "INTERFACE_LIBRARY")
      message(FATAL_ERROR
        "F4_PORTABLE_STATIC requires static QWindowKit; QWindowKit::Quick is ${F4_QWK_QUICK_TYPE}")
    endif()
  endif()
endif()

get_target_property(F4_QT_CORE_TARGET_TYPE Qt6::Core TYPE)
set(F4_QT_STATIC OFF)
if(F4_PORTABLE_STATIC OR
   F4_QT_CORE_TARGET_TYPE STREQUAL "STATIC_LIBRARY")
  set(F4_QT_STATIC ON)
endif()

set(F4_ZOINGALLERY_SOURCE_DIR
  "${CMAKE_CURRENT_LIST_DIR}/../../third_party/ZoinGallery")
if(NOT EXISTS "${F4_ZOINGALLERY_SOURCE_DIR}/CMakeLists.txt")
  message(FATAL_ERROR
    "ZoinGallery submodule is missing. Run: git submodule update --init --recursive")
endif()

# ZoinGallery is pinned by the f4 Git tree and participates in the same CMake
# graph. Conan supplies its third-party dependencies, not a separately built
# zoingallery package.
set(ZOIN_BUILD_STANDALONE OFF CACHE BOOL
  "Build the standalone ZoinGallery application" FORCE)
set(ZOIN_BUILD_BENCHMARKS OFF CACHE BOOL
  "Build ZoinGallery performance audit tools" FORCE)
set(ZOIN_ENABLE_INSTALL OFF CACHE BOOL
  "Generate ZoinGallery package installation rules" FORCE)
set(F4_ZOINGALLERY_BINARY_DIR
  "${CMAKE_CURRENT_BINARY_DIR}/ZoinGallery")

if(F4_PORTABLE_STATIC AND UNIX AND NOT APPLE)
  # Conan's static packages currently expose system C++ runtime libraries as
  # literal "stdc++" interface entries. Those entries are emitted as an
  # explicit dynamic -lstdc++ after the compiler driver's
  # -static-libstdc++ selection, defeating the portable host contract. Remove
  # only that metadata entry; the compiler still adds its static C++ runtime
  # because the host's link options request it. Recurse over the submodule
  # directory too, since its find_package calls own part of the dependency
  # graph.
  function(f4_strip_static_cpp_runtime_metadata directory)
    get_property(F4_CONAN_IMPORTED_TARGETS
      DIRECTORY "${directory}" PROPERTY IMPORTED_TARGETS)
    get_property(F4_CONAN_BUILDSYSTEM_TARGETS
      DIRECTORY "${directory}" PROPERTY BUILDSYSTEM_TARGETS)
    list(APPEND F4_CONAN_IMPORTED_TARGETS ${F4_CONAN_BUILDSYSTEM_TARGETS})
    foreach(F4_CONAN_IMPORTED_TARGET IN LISTS F4_CONAN_IMPORTED_TARGETS)
      if(TARGET ${F4_CONAN_IMPORTED_TARGET})
        get_target_property(F4_CONAN_LINK_LIBRARIES
          ${F4_CONAN_IMPORTED_TARGET} INTERFACE_LINK_LIBRARIES)
        if(F4_CONAN_LINK_LIBRARIES)
          string(REPLACE "stdc++" "" F4_CONAN_LINK_LIBRARIES
            "${F4_CONAN_LINK_LIBRARIES}")
          set_target_properties(${F4_CONAN_IMPORTED_TARGET} PROPERTIES
            INTERFACE_LINK_LIBRARIES "${F4_CONAN_LINK_LIBRARIES}")
        endif()
      endif()
    endforeach()
    get_property(F4_CONAN_SUBDIRECTORIES
      DIRECTORY "${directory}" PROPERTY SUBDIRECTORIES)
    foreach(F4_CONAN_SUBDIRECTORY IN LISTS F4_CONAN_SUBDIRECTORIES)
      f4_strip_static_cpp_runtime_metadata("${F4_CONAN_SUBDIRECTORY}")
    endforeach()
  endfunction()

  # The callback is deferred from ZoinGallery's own project directory so its
  # directory-scoped imported Conan targets remain visible to TARGET checks.
  set(F4_PREVIOUS_CMAKE_PROJECT_INCLUDE "${CMAKE_PROJECT_INCLUDE}")
  set(CMAKE_PROJECT_INCLUDE
    "${CMAKE_CURRENT_SOURCE_DIR}/cmake/f4-zoin-gallery-portable-hook.cmake")
endif()
add_subdirectory(
  "${F4_ZOINGALLERY_SOURCE_DIR}"
  "${F4_ZOINGALLERY_BINARY_DIR}")
if(F4_PORTABLE_STATIC AND UNIX AND NOT APPLE)
  set(CMAKE_PROJECT_INCLUDE "${F4_PREVIOUS_CMAKE_PROJECT_INCLUDE}")
  unset(F4_PREVIOUS_CMAKE_PROJECT_INCLUDE)
endif()
if(NOT TARGET ZoinGallery::Core OR NOT TARGET ZoinGallery::Qml)
  message(FATAL_ERROR
    "The bundled ZoinGallery submodule must provide Core and Qml targets")
endif()
set(ZoinGallery_QML_IMPORT_PATH "${F4_ZOINGALLERY_BINARY_DIR}")
if(F4_PORTABLE_STATIC AND UNIX AND NOT APPLE)
  f4_strip_static_cpp_runtime_metadata("${CMAKE_CURRENT_SOURCE_DIR}")
endif()

function(f4_link_static_zoingallery_plugin target)
  if(NOT TARGET ZoinGalleryQmlplugin)
    return()
  endif()
  get_target_property(F4_ZOIN_QML_PLUGIN_TYPE ZoinGalleryQmlplugin TYPE)
  if(F4_ZOIN_QML_PLUGIN_TYPE STREQUAL "STATIC_LIBRARY")
    target_link_libraries(${target} PRIVATE ZoinGalleryQmlplugin)
    if(TARGET ZoinGalleryQmlplugin_init)
      target_link_libraries(${target} PRIVATE ZoinGalleryQmlplugin_init)
    endif()
  endif()
endfunction()

# ZoinGallery's own QML-facing tests are created by add_subdirectory() before
# this point.  A static QML module has no loadable plugin file beside qmldir,
# so each executable must carry both the plugin and its generated Q_IMPORT
# initializer.  Otherwise QQmlEngine rejects the in-tree module and may fall
# through to an unrelated ZoinGallery installation on the import path.
foreach(F4_ZOIN_QML_TEST_TARGET IN ITEMS
    ZoinGallerySessionTests
    ZoinGalleryQmlInteractionTests
    ZoinGalleryReusableViewerPrimitivesTests
    ZoinGalleryPanelGestureTests
    ZoinGalleryMasonryLayoutModesTests
    ZoinGalleryStandaloneShellSmokeTests
    ZoinGalleryLayoutInteractionBenchmark)
  if(TARGET ${F4_ZOIN_QML_TEST_TARGET})
    f4_link_static_zoingallery_plugin(${F4_ZOIN_QML_TEST_TARGET})
  endif()
endforeach()

if(COMMAND qt_policy)
  qt_policy(SET QTP0004 NEW)
endif()

set(F4_QT_HOST_SOURCES
  "${CMAKE_CURRENT_SOURCE_DIR}/src/main.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/DummyQWK.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/QtShellController.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/QtShellController.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/QtMediaClient.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/QtMediaClient.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4ImageSourceProvider.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4ImageSourceProvider.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4GalleryBridge.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4GalleryBridge.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4IconProvider.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4IconProvider.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/WindowGeometryPersistence.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/WindowGeometryPersistence.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4ThemePersistence.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4ThemePersistence.cpp"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4TextRenderingPolicy.h"
  "${CMAKE_CURRENT_SOURCE_DIR}/src/F4TextRenderingPolicy.cpp"
)

if(APPLE)
  list(APPEND F4_QT_HOST_SOURCES
    "${CMAKE_CURRENT_SOURCE_DIR}/src/MacApplicationMenu.h"
    "${CMAKE_CURRENT_SOURCE_DIR}/src/MacApplicationMenu.mm"
    "${CMAKE_CURRENT_SOURCE_DIR}/src/MacPlatformServices.h"
    "${CMAKE_CURRENT_SOURCE_DIR}/src/MacPlatformServices.mm"
  )
endif()

set(F4_LUCIDE_ICON_NAMES
  archive.svg
  arrow-down.svg
  arrow-down-a-z.svg
  arrow-down-wide-narrow.svg
  arrow-up.svg
  binary.svg
  book-open.svg
  check.svg
  circle-check.svg
  circle-play.svg
  circle-question-mark.svg
  circle-x.svg
  chevron-down.svg
  chevron-right.svg
  clock-3.svg
  cloud.svg
  columns-2.svg
  columns-3.svg
  copy.svg
  database.svg
  eye.svg
  file.svg
  file-clock.svg
  file-code.svg
  file-cog.svg
  file-lock.svg
  file-pen-line.svg
  file-plus.svg
  file-text.svg
  file-type.svg
  flask-conical.svg
  folder.svg
  folder-clock.svg
  folder-input.svg
  folder-kanban.svg
  folder-lock.svg
  folder-plus.svg
  folder-root.svg
  folder-up.svg
  git-fork.svg
  globe.svg
  image.svg
  images.svg
  grid-3x3.svg
  hard-drive.svg
  house.svg
  layout-dashboard.svg
  languages.svg
  library.svg
  list.svg
  list-checks.svg
  list-restart.svg
  list-tree.svg
  loader-circle.svg
  locate-fixed.svg
  log-out.svg
  maximize-2.svg
  menu.svg
  monitor.svg
  music.svg
  network.svg
  panels-top-left.svg
  panel-left.svg
  panel-right.svg
  palette.svg
  pencil.svg
  plug.svg
  plus.svg
  refresh-cw.svg
  rotate-ccw.svg
  rows-3.svg
  rows-4.svg
  save.svg
  search.svg
  sparkles.svg
  space.svg
  square-terminal.svg
  text-wrap.svg
  trash-2.svg
  triangle-alert.svg
  video.svg
  x.svg
)
set(F4_LUCIDE_ICONS ${F4_LUCIDE_ICON_NAMES})
list(TRANSFORM F4_LUCIDE_ICONS PREPEND "${CMAKE_CURRENT_SOURCE_DIR}/icons/lucide/")

set(F4_STREAMLINE_ICON_NAMES
  android-logo.svg
  apple-logo.svg
)
set(F4_STREAMLINE_ICONS ${F4_STREAMLINE_ICON_NAMES})
list(TRANSFORM F4_STREAMLINE_ICONS PREPEND "${CMAKE_CURRENT_SOURCE_DIR}/icons/streamline/")

# Gallery renders the same icons at a much larger size. Generate a second,
# thinner set from the vendored Lucide SVGs at configure time so the two sets
# cannot drift and no SVG rewriting is needed at runtime.
set(F4_GALLERY_LUCIDE_STROKE_WIDTH "1" CACHE STRING
  "Stroke width used by the large Lucide icons in Gallery")
set(F4_GALLERY_LUCIDE_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated/lucide-gallery")
file(MAKE_DIRECTORY "${F4_GALLERY_LUCIDE_DIR}")
set(F4_GALLERY_LUCIDE_ICONS)
foreach(icon_name IN LISTS F4_LUCIDE_ICON_NAMES)
  set(icon_source "${CMAKE_CURRENT_SOURCE_DIR}/icons/lucide/${icon_name}")
  set(icon_output "${F4_GALLERY_LUCIDE_DIR}/${icon_name}")
  set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS "${icon_source}")
  file(READ "${icon_source}" icon_svg)
  string(REPLACE "stroke-width=\"2\""
    "stroke-width=\"${F4_GALLERY_LUCIDE_STROKE_WIDTH}\""
    gallery_icon_svg "${icon_svg}")
  if(gallery_icon_svg STREQUAL icon_svg)
    message(FATAL_ERROR "Lucide icon ${icon_name} has no expected stroke-width=\"2\"")
  endif()
  file(WRITE "${icon_output}" "${gallery_icon_svg}")
  list(APPEND F4_GALLERY_LUCIDE_ICONS "${icon_output}")
endforeach()

# Static imports are explicit, so evaluate the target here to keep MSVC/Ninja
# object paths rooted at the top-level build directory. Shared builds retain a
# clean scanner root so unrelated build/test QML cannot pollute deployment.
if(F4_QT_STATIC)
  include(cmake/f4-qt-host-target/CMakeLists.txt)
else()
  add_subdirectory(cmake/f4-qt-host-target)
endif()

if(F4_PORTABLE_STATIC AND UNIX AND NOT APPLE)
  # The host target can cause Qt's generated package facades to be completed
  # while its transitive link interface is assembled.  Strip once more after
  # that evaluation so a late Conan system-library entry cannot reintroduce a
  # shared libstdc++ dependency after -static-libstdc++.
  f4_strip_static_cpp_runtime_metadata("${CMAKE_CURRENT_SOURCE_DIR}")
endif()

# ZoinGallery's QML tests are created before the host target is included, so
# they cannot use the static-test helpers declared in the BUILD_TESTING block
# below.  Give their executables the same self-contained Qt platform and QML
# plugin set as the production host; otherwise a static test falls back to
# runtime plugin discovery and can block on Qt's platform-plugin error dialog.
if(F4_QT_STATIC)
  foreach(F4_ZOIN_STATIC_QML_TEST_TARGET IN ITEMS
      ZoinGallerySessionTests
      ZoinGalleryQmlInteractionTests
      ZoinGalleryReusableViewerPrimitivesTests
      ZoinGalleryPanelGestureTests
      ZoinGalleryMasonryLayoutModesTests
      ZoinGalleryStandaloneShellSmokeTests)
    if(TARGET ${F4_ZOIN_STATIC_QML_TEST_TARGET})
      target_sources(${F4_ZOIN_STATIC_QML_TEST_TARGET} PRIVATE
        "${CMAKE_CURRENT_SOURCE_DIR}/tests/StaticQtTestPluginImports.cpp"
        "${CMAKE_CURRENT_SOURCE_DIR}/tests/StaticQtImagePluginImports.cpp"
        "${CMAKE_CURRENT_SOURCE_DIR}/src/StaticQmlPluginImports.cpp")
      target_link_libraries(${F4_ZOIN_STATIC_QML_TEST_TARGET} PRIVATE
        ${F4_REQUIRED_STATIC_QT_PLUGINS}
        ${F4_REQUIRED_STATIC_QML_PLUGINS}
        "$<LINK_GROUP:RESCAN,CONAN_LIB::qt_Qt6_Svg_Qt6Svg_RELEASE,CONAN_LIB::qt_Qt6_QSvgPlugin_qsvg_RELEASE>"
        "$<LINK_LIBRARY:WHOLE_ARCHIVE,Qt6::QuickControls2Basic>")
    endif()
  endforeach()
endif()

# The Core-only ZoinGallery tests use QTEST_MAIN, which selects a GUI
# application when the library graph exposes QtGui.  Static Qt cannot discover
# the offscreen platform or image-format plugins at runtime, so carry those
# plugin imports into the tests that the submodule creates before this file's
# BUILD_TESTING helpers are declared.
if(F4_QT_STATIC)
  foreach(F4_ZOIN_STATIC_CORE_TEST_TARGET IN ITEMS
      ZoinGallerySelectionTests
      ZoinGalleryViewerCacheTests
      ZoinGalleryThumbnailMemoryCacheTests
      ZoinGalleryEmbeddedImageProbeTests
      ZoinGalleryDecodeSizePolicyTests
      ZoinGalleryPersistentDerivedImageCacheTests
      ZoinGalleryDecodeLifecycleTests)
    if(TARGET ${F4_ZOIN_STATIC_CORE_TEST_TARGET})
      target_sources(${F4_ZOIN_STATIC_CORE_TEST_TARGET} PRIVATE
        "${CMAKE_CURRENT_SOURCE_DIR}/tests/StaticQtTestPluginImports.cpp"
        "${CMAKE_CURRENT_SOURCE_DIR}/tests/StaticQtImagePluginImports.cpp")
      target_link_libraries(${F4_ZOIN_STATIC_CORE_TEST_TARGET} PRIVATE
        Qt6::QOffscreenIntegrationPlugin
        Qt6::QGifPlugin
        Qt6::QIcoPlugin
        Qt6::QJpegPlugin
        Qt6::QSvgPlugin
        "$<LINK_GROUP:RESCAN,CONAN_LIB::qt_Qt6_Svg_Qt6Svg_RELEASE,CONAN_LIB::qt_Qt6_QSvgPlugin_qsvg_RELEASE>")
    endif()
  endforeach()
endif()

if(F4_PORTABLE_STATIC)
  if(MSVC)
    set_property(TARGET f4-qt-host PROPERTY
      MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
  elseif(UNIX AND NOT APPLE)
    target_compile_options(f4-qt-host PRIVATE
      -fPIE -ffunction-sections -fdata-sections -fstack-protector-strong)
    target_link_options(f4-qt-host PRIVATE
      -pie
      -static-libstdc++
      -static-libgcc
      -Wl,--gc-sections
      -Wl,--exclude-libs,ALL
      -Wl,-z,relro,-z,now,-z,noexecstack)
  endif()
endif()

install(TARGETS f4-qt-host RUNTIME DESTINATION bin)
if(APPLE)
  install(DIRECTORY
    "${CMAKE_BINARY_DIR}/bin/$<CONFIG>/f4-qt-host.app"
    DESTINATION bin
    USE_SOURCE_PERMISSIONS
  )
endif()
install(FILES icons/lucide/LICENSE
  DESTINATION licenses
  RENAME Lucide-LICENSE.txt
)
if(USE_QWK AND NOT F4_QT_STATIC)
  install(IMPORTED_RUNTIME_ARTIFACTS
    QWindowKit::Core
    QWindowKit::Quick
    RUNTIME DESTINATION bin
    LIBRARY DESTINATION lib
  )
endif()

if(APPLE)
  configure_file(packaging/qt.conf.in "${CMAKE_BINARY_DIR}/qt.conf" @ONLY)
  install(FILES "${CMAKE_BINARY_DIR}/qt.conf" DESTINATION bin)
  install(DIRECTORY "${CMAKE_BINARY_DIR}/lib/$<CONFIG>/"
    DESTINATION lib
    USE_SOURCE_PERMISSIONS
  )
  install(DIRECTORY "${CMAKE_BINARY_DIR}/qml/"
    DESTINATION qml
    USE_SOURCE_PERMISSIONS
  )
  install(DIRECTORY "${CMAKE_BINARY_DIR}/plugins/"
    DESTINATION plugins
    USE_SOURCE_PERMISSIONS
  )
elseif(NOT F4_QT_STATIC)
  qt_generate_deploy_qml_app_script(
    TARGET f4-qt-host
    OUTPUT_SCRIPT F4_QT_HOST_DEPLOY_SCRIPT
  )
  # Conan's Qt package intentionally leaves Qt's original install layout
  # variables empty. Supply the sidecar layout explicitly so deployment does
  # not fail with an empty QML_DIR and emits a relocatable qt.conf.
  install(CODE "
    set(QT_DEPLOY_BIN_DIR bin)
    set(QT_DEPLOY_LIBEXEC_DIR bin)
    set(QT_DEPLOY_LIB_DIR lib)
    set(QT_DEPLOY_PLUGINS_DIR plugins)
    set(QT_DEPLOY_QML_DIR qml)
    set(QT_DEPLOY_TRANSLATIONS_DIR translations)
    include(\"${F4_QT_HOST_DEPLOY_SCRIPT}\")
  ")
endif()

if(BUILD_TESTING)
  find_package(Qt6 COMPONENTS Test REQUIRED)

  if(APPLE)
    add_executable(MacApplicationMenuTests
      tests/MacApplicationMenuTests.mm
      src/MacApplicationMenu.h
      src/MacApplicationMenu.mm
    )
    target_include_directories(MacApplicationMenuTests PRIVATE
      ${CMAKE_CURRENT_SOURCE_DIR}/src
    )
    target_link_libraries(MacApplicationMenuTests PRIVATE
      Qt6::Core
      Qt6::Test
      "-framework AppKit"
    )
    add_test(NAME MacApplicationMenuTest COMMAND MacApplicationMenuTests)

    add_executable(MacPlatformServicesTests
      tests/MacPlatformServicesTests.mm
    )
    target_include_directories(MacPlatformServicesTests PRIVATE
      ${CMAKE_CURRENT_SOURCE_DIR}/src
    )
    target_link_libraries(MacPlatformServicesTests PRIVATE
      Qt6::Core
      Qt6::Test
      "-framework AppKit"
      "-framework NetFS"
      "-framework Network"
    )
    add_test(NAME MacPlatformServicesTest COMMAND MacPlatformServicesTests)
  endif()

  function(f4_add_icon_test_resources target)
    qt6_add_resources(${target} ${target}_f4_lucide_icons
      PREFIX "/F4QtHost/icons/lucide"
      BASE "${CMAKE_CURRENT_SOURCE_DIR}/icons/lucide"
      FILES ${F4_LUCIDE_ICONS}
    )
    qt6_add_resources(${target} ${target}_f4_gallery_lucide_icons
      PREFIX "/F4QtHost/icons/lucide-gallery"
      BASE "${F4_GALLERY_LUCIDE_DIR}"
      FILES ${F4_GALLERY_LUCIDE_ICONS}
    )
    qt6_add_resources(${target} ${target}_f4_streamline_icons
      PREFIX "/F4QtHost/icons/streamline"
      BASE "${CMAKE_CURRENT_SOURCE_DIR}/icons/streamline"
      FILES ${F4_STREAMLINE_ICONS}
    )
    qt6_add_resources(${target} ${target}_f4_app_icon
      PREFIX "/F4QtHost/icons/app"
      BASE "${CMAKE_CURRENT_SOURCE_DIR}/../../assets/icon"
      FILES "${CMAKE_CURRENT_SOURCE_DIR}/../../assets/icon/f4.svg"
    )
  endfunction()

  if(TARGET ZoinGalleryMasonryLayoutModesTests)
    f4_add_icon_test_resources(ZoinGalleryMasonryLayoutModesTests)
    add_test(NAME ZoinGalleryLucideTintFractionalDprTest
      COMMAND ZoinGalleryMasonryLayoutModesTests
        columnsAndDetailsUseIdenticalCompactLucideIcons)
    set_tests_properties(ZoinGalleryLucideTintFractionalDprTest PROPERTIES
      ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
    )
  endif()

  function(f4_link_static_test_platform target)
    if(F4_QT_STATIC)
      target_sources(${target} PRIVATE
        tests/StaticQtTestPluginImports.cpp)
      target_link_libraries(${target} PRIVATE
        Qt6::QOffscreenIntegrationPlugin)
      if(WIN32)
        target_link_libraries(${target} PRIVATE
          Qt6::QWindowsIntegrationPlugin)
      endif()
    endif()
  endfunction()

  function(f4_link_static_test_qml_plugins target)
    if(F4_QT_STATIC)
      target_sources(${target} PRIVATE
        src/StaticQmlPluginImports.cpp)
      target_link_libraries(${target} PRIVATE
        ${F4_REQUIRED_STATIC_QML_PLUGINS}
        "$<LINK_LIBRARY:WHOLE_ARCHIVE,Qt6::QuickControls2Basic>")
    endif()
  endfunction()

  function(f4_link_static_test_image_plugins target)
    if(F4_QT_STATIC)
      target_sources(${target} PRIVATE
        tests/StaticQtImagePluginImports.cpp)
      target_link_libraries(${target} PRIVATE
        Qt6::QGifPlugin
        Qt6::QIcoPlugin
        Qt6::QJpegPlugin
        Qt6::QSvgPlugin)
    endif()
  endfunction()

  if(USE_QWK AND UNIX AND NOT APPLE)
    add_executable(QWindowKitTitleBarTests
      tests/QWindowKitTitleBarTests.cpp
    )
    target_link_libraries(QWindowKitTitleBarTests PRIVATE
      Qt6::Core
      Qt6::Gui
      Qt6::Quick
      Qt6::Test
      QWindowKit::Quick
    )
    f4_link_static_test_platform(QWindowKitTitleBarTests)
    add_test(NAME QWindowKitTitleBarTest COMMAND QWindowKitTitleBarTests)
    set_tests_properties(QWindowKitTitleBarTest PROPERTIES
      ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
    )
  endif()

  add_executable(F4IconProviderTests
    tests/F4IconProviderTests.cpp
    src/F4IconProvider.h
    src/F4IconProvider.cpp
  )
  target_include_directories(F4IconProviderTests PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
  f4_add_icon_test_resources(F4IconProviderTests)
  target_link_libraries(F4IconProviderTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::Svg
    Qt6::Test
  )
  f4_link_static_test_platform(F4IconProviderTests)
  f4_link_static_test_qml_plugins(F4IconProviderTests)
  add_test(NAME F4IconProviderTest COMMAND F4IconProviderTests)
  set_tests_properties(F4IconProviderTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4IconProviderFractionalDprFramebufferTest
    COMMAND F4IconProviderTests lucideFramebufferMatchesDirectSvgRender)
  set_tests_properties(F4IconProviderFractionalDprFramebufferTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  # Windows CI runs in headless Session 0, where the Shell file-icon provider
  # can block indefinitely. The PE audit plus the offscreen production-host
  # smoke cover the static Windows QPA path; exercise real desktop icons on
  # macOS, where the runner has a native window-server session.
  if(APPLE)
    add_test(NAME F4IconProviderNativeSmoke
      COMMAND F4IconProviderTests nativePlatformFileIconSmoke)
  set_tests_properties(F4IconProviderNativeSmoke PROPERTIES
      ENVIRONMENT "QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
    )
  endif()

  add_executable(F4TextRenderingPolicyTests
    tests/F4TextRenderingPolicyTests.cpp
    src/F4TextRenderingPolicy.h
    src/F4TextRenderingPolicy.cpp
  )
  target_include_directories(F4TextRenderingPolicyTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  target_link_libraries(F4TextRenderingPolicyTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::Test
  )
  f4_link_static_test_platform(F4TextRenderingPolicyTests)
  f4_link_static_test_qml_plugins(F4TextRenderingPolicyTests)
  add_test(NAME F4TextRenderingPolicyTest COMMAND F4TextRenderingPolicyTests)
  set_tests_properties(F4TextRenderingPolicyTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4PanelSplitterTests
    tests/F4PanelSplitterTests.cpp
  )
  qt6_add_resources(F4PanelSplitterTests f4_panel_splitter_test_qml
    PREFIX "/F4QtHost"
    FILES qml/PanelSplitter.qml
  )
  target_link_libraries(F4PanelSplitterTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::Test
  )
  f4_link_static_test_platform(F4PanelSplitterTests)
  f4_link_static_test_qml_plugins(F4PanelSplitterTests)
  add_test(NAME F4PanelSplitterTest COMMAND F4PanelSplitterTests)
  set_tests_properties(F4PanelSplitterTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4DocumentSurfaceTests
    tests/F4DocumentSurfaceTests.cpp
    src/DummyQWK.h
  )
  target_include_directories(F4DocumentSurfaceTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  qt6_add_resources(F4DocumentSurfaceTests f4_document_surface_test_qml
    PREFIX "/F4QtHost"
    FILES
      qml/main.qml
      qml/PanelSplitter.qml
  )
  f4_add_icon_test_resources(F4DocumentSurfaceTests)
  target_link_libraries(F4DocumentSurfaceTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Test
    ZoinGallery::Qml
  )
  f4_link_static_zoingallery_plugin(F4DocumentSurfaceTests)
  f4_link_static_test_platform(F4DocumentSurfaceTests)
  f4_link_static_test_qml_plugins(F4DocumentSurfaceTests)
  add_test(NAME F4DocumentSurfaceTest COMMAND F4DocumentSurfaceTests)
  set_tests_properties(F4DocumentSurfaceTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4OperationsQueueTests
    tests/F4OperationsQueueTests.cpp
    src/DummyQWK.h
  )
  target_include_directories(F4OperationsQueueTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  qt6_add_resources(F4OperationsQueueTests f4_operations_queue_test_qml
    PREFIX "/F4QtHost"
    FILES
      qml/main.qml
      qml/PanelSplitter.qml
  )
  f4_add_icon_test_resources(F4OperationsQueueTests)
  target_link_libraries(F4OperationsQueueTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Test
    ZoinGallery::Qml
  )
  f4_link_static_zoingallery_plugin(F4OperationsQueueTests)
  f4_link_static_test_platform(F4OperationsQueueTests)
  f4_link_static_test_qml_plugins(F4OperationsQueueTests)
  add_test(NAME F4OperationsQueueTest COMMAND F4OperationsQueueTests)
  set_tests_properties(F4OperationsQueueTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4QuickViewSurfaceTests
    tests/F4QuickViewSurfaceTests.cpp
    src/DummyQWK.h
    src/F4TextRenderingPolicy.h
    src/F4TextRenderingPolicy.cpp
  )
  target_include_directories(F4QuickViewSurfaceTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  qt6_add_resources(F4QuickViewSurfaceTests f4_quick_view_surface_test_qml
    PREFIX "/F4QtHost"
    FILES
      qml/main.qml
      qml/PanelSplitter.qml
      tests/TestGalleryPanel.qml
  )
  f4_add_icon_test_resources(F4QuickViewSurfaceTests)
  target_link_libraries(F4QuickViewSurfaceTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Svg
    Qt6::Test
    ZoinGallery::Qml
  )
  f4_link_static_zoingallery_plugin(F4QuickViewSurfaceTests)
  f4_link_static_test_platform(F4QuickViewSurfaceTests)
  f4_link_static_test_qml_plugins(F4QuickViewSurfaceTests)
  f4_link_static_test_image_plugins(F4QuickViewSurfaceTests)
  add_test(NAME F4QuickViewSurfaceTest COMMAND F4QuickViewSurfaceTests)
  set_tests_properties(F4QuickViewSurfaceTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewIconPixelGridTest
    COMMAND F4QuickViewSurfaceTests chromeIconsUseMatchingPhysicalTargetSizes)
  set_tests_properties(F4QuickViewIconPixelGridTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewPanelDriveButtonTest
    COMMAND F4QuickViewSurfaceTests
      panelDriveButtonUsesPathIconAndRequestsDriveMenu)
  set_tests_properties(F4QuickViewPanelDriveButtonTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewDriveMenuIconsTest
    COMMAND F4QuickViewSurfaceTests
      driveMenuIconsUseSemanticModelAndLiveTheme)
  set_tests_properties(F4QuickViewDriveMenuIconsTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewNestedMenuPixelGridTest
    COMMAND F4QuickViewSurfaceTests
      nestedMenuAnchorsHeadersTagsAndChevronsOnPhysicalPixels)
  set_tests_properties(F4QuickViewNestedMenuPixelGridTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewNestedMenuHoverTest
    COMMAND F4QuickViewSurfaceTests nestedMenuHoverUsesDelayedSubmenuAction)
  set_tests_properties(F4QuickViewNestedMenuHoverTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewTextPixelGridTest
    COMMAND F4QuickViewSurfaceTests
      workspaceTabTextParentsStayOnPhysicalPixelGrid)
  set_tests_properties(F4QuickViewTextPixelGridTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewPathBreadcrumbPixelGridTest
    COMMAND F4QuickViewSurfaceTests
      pathBreadcrumbTextStaysFixedWhenNavigatingDeeper)
  set_tests_properties(F4QuickViewPathBreadcrumbPixelGridTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  add_test(NAME F4QuickViewThemeDialogPixelGridTest
    COMMAND F4QuickViewSurfaceTests
      themeDialogControlsStayOnPhysicalPixelGridAt175Percent)
  set_tests_properties(F4QuickViewThemeDialogPixelGridTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_SCALE_FACTOR=1.75;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(QtShellControllerTests
    tests/QtShellControllerTests.cpp
    src/QtShellController.h
    src/QtShellController.cpp
  )
  target_include_directories(QtShellControllerTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  target_link_libraries(QtShellControllerTests PRIVATE
    Qt6::Core
    Qt6::Network
    Qt6::Test
  )
  target_compile_definitions(QtShellControllerTests PRIVATE
    F4_QT_HOST_TEST_EXECUTABLE="$<TARGET_FILE:f4-qt-host>"
  )

  # Conan's CMakeDeps generator represents shared package components with an
  # INTERFACE facade which links an internal imported dylib target. Such a
  # facade has no TARGET_FILE_DIR of its own. The built host already carries
  # the required rpaths on these configurations, so leave the optional test
  # PATH hint empty instead of evaluating an invalid generator expression.
  function(f4_test_runtime_directory out_variable target)
    get_target_property(target_type ${target} TYPE)
    if(target_type STREQUAL "INTERFACE_LIBRARY")
      set(${out_variable} "" PARENT_SCOPE)
    else()
      set(${out_variable} "$<TARGET_FILE_DIR:${target}>" PARENT_SCOPE)
    endif()
  endfunction()
  if(F4_QT_STATIC)
    target_compile_definitions(QtShellControllerTests PRIVATE
      F4_QT_RUNTIME_DIRECTORY=""
    )
  else()
    f4_test_runtime_directory(F4_QT_CORE_RUNTIME_DIRECTORY Qt6::Core)
    target_compile_definitions(QtShellControllerTests PRIVATE
      F4_QT_RUNTIME_DIRECTORY="${F4_QT_CORE_RUNTIME_DIRECTORY}"
    )
  endif()
  if(USE_QWK)
    f4_test_runtime_directory(F4_QWK_RUNTIME_DIRECTORY QWindowKit::Quick)
    target_compile_definitions(QtShellControllerTests PRIVATE
      F4_QWK_RUNTIME_DIRECTORY="${F4_QWK_RUNTIME_DIRECTORY}"
    )
  endif()
  if(TARGET libraw::libraw_)
    f4_test_runtime_directory(F4_LIBRAW_RUNTIME_DIRECTORY libraw::libraw_)
    target_compile_definitions(QtShellControllerTests PRIVATE
      F4_LIBRAW_RUNTIME_DIRECTORY="${F4_LIBRAW_RUNTIME_DIRECTORY}"
    )
  endif()
  add_dependencies(QtShellControllerTests f4-qt-host)
  if(TARGET msgpack-cxx)
    target_link_libraries(QtShellControllerTests PRIVATE msgpack-cxx)
  elseif(TARGET msgpack-cxx::msgpack-cxx)
    target_link_libraries(QtShellControllerTests PRIVATE msgpack-cxx::msgpack-cxx)
  endif()
  add_test(NAME QtShellControllerTest COMMAND QtShellControllerTests)

  add_executable(QtMediaClientTests
    tests/QtMediaClientTests.cpp
    src/QtMediaClient.h
    src/QtMediaClient.cpp
    src/QtShellController.h
    src/QtShellController.cpp
  )
  target_include_directories(QtMediaClientTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  target_compile_definitions(QtMediaClientTests PRIVATE
    F4_MEDIA_CLIENT_TESTING=1
  )
  target_link_libraries(QtMediaClientTests PRIVATE
    Qt6::Core
    Qt6::Network
    Qt6::Test
    ZoinGallery::Core
  )
  if(TARGET msgpack-cxx)
    target_link_libraries(QtMediaClientTests PRIVATE msgpack-cxx)
  elseif(TARGET msgpack-cxx::msgpack-cxx)
    target_link_libraries(QtMediaClientTests PRIVATE msgpack-cxx::msgpack-cxx)
  endif()
  add_test(NAME QtMediaClientTest COMMAND QtMediaClientTests)

  add_executable(WindowGeometryPersistenceTests
    tests/WindowGeometryPersistenceTests.cpp
    src/WindowGeometryPersistence.h
    src/WindowGeometryPersistence.cpp
  )
  target_include_directories(WindowGeometryPersistenceTests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
  )
  target_link_libraries(WindowGeometryPersistenceTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Test
  )
  f4_link_static_test_platform(WindowGeometryPersistenceTests)
  add_test(NAME WindowGeometryPersistenceTest
    COMMAND WindowGeometryPersistenceTests)
  set_tests_properties(WindowGeometryPersistenceTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4GalleryBridgeTests
    tests/F4GalleryBridgeTests.cpp
    src/QtMediaClient.h
    src/QtMediaClient.cpp
    src/F4ImageSourceProvider.h
    src/F4ImageSourceProvider.cpp
    src/F4GalleryBridge.h
    src/F4GalleryBridge.cpp
    src/F4IconProvider.h
    src/F4IconProvider.cpp
  )
  target_include_directories(F4GalleryBridgeTests PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
  f4_add_icon_test_resources(F4GalleryBridgeTests)
  target_link_libraries(F4GalleryBridgeTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Network
    Qt6::Svg
    Qt6::Test
  )
  qt6_add_resources(F4GalleryBridgeTests f4_gallery_bridge_test_qml
    PREFIX "/F4QtHost"
    FILES
      qml/GalleryPanelHost.qml
      qml/GalleryViewerHost.qml
  )
  target_link_libraries(F4GalleryBridgeTests PRIVATE
    ZoinGallery::Core
    ZoinGallery::Qml
  )
  if(TARGET msgpack-cxx)
    target_link_libraries(F4GalleryBridgeTests PRIVATE msgpack-cxx)
  elseif(TARGET msgpack-cxx::msgpack-cxx)
    target_link_libraries(F4GalleryBridgeTests PRIVATE msgpack-cxx::msgpack-cxx)
  endif()
  f4_link_static_zoingallery_plugin(F4GalleryBridgeTests)
  f4_link_static_test_platform(F4GalleryBridgeTests)
  f4_link_static_test_qml_plugins(F4GalleryBridgeTests)
  add_test(NAME F4GalleryBridgeTest COMMAND F4GalleryBridgeTests)
  set_tests_properties(F4GalleryBridgeTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins;QML_IMPORT_PATH=${ZoinGallery_QML_IMPORT_PATH}"
  )

  add_executable(F4GalleryPointerTests
    tests/F4GalleryPointerTests.cpp
    src/QtMediaClient.h
    src/QtMediaClient.cpp
    src/F4ImageSourceProvider.h
    src/F4ImageSourceProvider.cpp
    src/F4GalleryBridge.h
    src/F4GalleryBridge.cpp
    src/F4IconProvider.h
    src/F4IconProvider.cpp
    src/VtuiGridItem.h
    src/VtuiGridItem.cpp
    src/QtShellController.h
    src/QtShellController.cpp
  )
  target_include_directories(F4GalleryPointerTests PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
  f4_add_icon_test_resources(F4GalleryPointerTests)
  qt6_add_resources(F4GalleryPointerTests f4_gallery_pointer_test_qml
    PREFIX "/F4QtHost"
    FILES
      qml/GalleryPanelHost.qml
      qml/GalleryViewerHost.qml
  )
  target_link_libraries(F4GalleryPointerTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Network
    Qt6::Svg
    Qt6::Test
    ZoinGallery::Core
    ZoinGallery::Qml
  )
  f4_link_static_zoingallery_plugin(F4GalleryPointerTests)
  f4_link_static_test_platform(F4GalleryPointerTests)
  f4_link_static_test_qml_plugins(F4GalleryPointerTests)
  if(TARGET msgpack-cxx)
    target_link_libraries(F4GalleryPointerTests PRIVATE msgpack-cxx)
  elseif(TARGET msgpack-cxx::msgpack-cxx)
    target_link_libraries(F4GalleryPointerTests PRIVATE msgpack-cxx::msgpack-cxx)
  endif()
  add_test(NAME F4GalleryPointerTest COMMAND F4GalleryPointerTests)
  set_tests_properties(F4GalleryPointerTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_QUICK_CONTROLS_STYLE=Basic;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins;QML_IMPORT_PATH=${ZoinGallery_QML_IMPORT_PATH}"
  )
endif()
