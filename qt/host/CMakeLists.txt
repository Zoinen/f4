cmake_minimum_required(VERSION 3.23)

if(POLICY CMP0091)
  cmake_policy(SET CMP0091 NEW)
endif()

set(CMAKE_OSX_DEPLOYMENT_TARGET "13.0" CACHE STRING "Minimum macOS deployment target" FORCE)

project(F4QtHost LANGUAGES CXX)

option(F4_WITH_ZOINGALLERY "Build the reusable ZoinGallery panel integration" OFF)
option(USE_QWK "Use QWindowKit" ON)
include(CTest)

if(NOT EXISTS "${CMAKE_BINARY_DIR}/conan_toolchain.cmake")
  message(FATAL_ERROR "The file conan_toolchain.cmake does not exist. Run: conan install . --build=missing -s build_type=RelWithDebInfo -s compiler.cppstd=20 --output-folder=build")
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 COMPONENTS Core Gui Qml Quick QuickControls2 Network Svg REQUIRED)
find_package(msgpack-cxx REQUIRED)
if(USE_QWK)
  find_package(QWindowKit REQUIRED)
endif()

if(F4_WITH_ZOINGALLERY)
  find_package(ZoinGallery CONFIG REQUIRED)
  if(NOT TARGET ZoinGallery::Core OR NOT TARGET ZoinGallery::Qml)
    message(FATAL_ERROR
      "F4_WITH_ZOINGALLERY requires the ZoinGallery::Core and ZoinGallery::Qml targets")
  endif()
endif()

if(COMMAND qt_policy)
  qt_policy(SET QTP0004 NEW)
endif()

set(F4_QT_HOST_SOURCES
  src/main.cpp
  src/DummyQWK.h
  src/QtShellController.h
  src/QtShellController.cpp
  src/F4GalleryBridge.h
  src/F4GalleryBridge.cpp
  src/F4IconProvider.h
  src/F4IconProvider.cpp
)

add_executable(f4-qt-host ${F4_QT_HOST_SOURCES})

target_include_directories(f4-qt-host PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)

target_compile_definitions(f4-qt-host PRIVATE
  F4_WITH_ZOINGALLERY=$<BOOL:${F4_WITH_ZOINGALLERY}>
  $<$<BOOL:${USE_QWK}>:__USE_QWK>
)

set_target_properties(f4-qt-host PROPERTIES
  RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin/$<CONFIG>"
)

if(APPLE)
  # The Go launcher expects a non-bundle executable. Keep all dylibs in the
  # sibling lib directory so the installed tree is relocatable.
  set_target_properties(f4-qt-host PROPERTIES
    INSTALL_RPATH "@loader_path/../lib"
  )
elseif(UNIX)
  set_target_properties(f4-qt-host PROPERTIES
    INSTALL_RPATH "$ORIGIN/../lib"
  )
endif()

set(F4_QT_HOST_QML_FILES
  qml/main.qml
  qml/PanelSplitter.qml
)
if(F4_WITH_ZOINGALLERY)
  list(APPEND F4_QT_HOST_QML_FILES
    qml/GalleryPanelHost.qml
    qml/GalleryViewerHost.qml
  )
endif()

qt6_add_qml_module(f4-qt-host
  URI F4QtHost
  VERSION 1.0
  RESOURCE_PREFIX /
  SOURCES
    src/VtuiGridItem.h
    src/VtuiGridItem.cpp
  QML_FILES ${F4_QT_HOST_QML_FILES}
)

set(F4_LUCIDE_ICON_NAMES
  archive.svg
  book-open.svg
  check.svg
  database.svg
  file.svg
  file-clock.svg
  file-code.svg
  file-cog.svg
  file-lock.svg
  file-text.svg
  folder.svg
  folder-kanban.svg
  folder-lock.svg
  folder-up.svg
  globe.svg
  image.svg
  library.svg
  music.svg
  square-terminal.svg
  video.svg
)
set(F4_LUCIDE_ICONS ${F4_LUCIDE_ICON_NAMES})
list(TRANSFORM F4_LUCIDE_ICONS PREPEND "${CMAKE_CURRENT_SOURCE_DIR}/icons/lucide/")
qt6_add_resources(f4-qt-host f4_lucide_icons
  PREFIX "/F4QtHost/icons/lucide"
  BASE "${CMAKE_CURRENT_SOURCE_DIR}/icons/lucide"
  FILES ${F4_LUCIDE_ICONS}
)

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
qt6_add_resources(f4-qt-host f4_gallery_lucide_icons
  PREFIX "/F4QtHost/icons/lucide-gallery"
  BASE "${F4_GALLERY_LUCIDE_DIR}"
  FILES ${F4_GALLERY_LUCIDE_ICONS}
)

# The Conan generator stages Qt's QML import tree here. Qt's Conan metadata
# does not publish its original QML prefix, so make the staged path explicit
# after qt_add_qml_module has initialized (and replaced) this target property.
set_property(TARGET f4-qt-host APPEND PROPERTY
  QT_QML_IMPORT_PATH "${CMAKE_BINARY_DIR}/qml"
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
if(USE_QWK)
  target_link_libraries(f4-qt-host PRIVATE QWindowKit::Quick)
endif()

if(F4_WITH_ZOINGALLERY)
  target_link_libraries(f4-qt-host PRIVATE
    ZoinGallery::Core
    ZoinGallery::Qml
  )
  # qmlimportscanner only reads import paths from the application target; it
  # does not traverse this property through linked imported targets.
  set_property(TARGET f4-qt-host APPEND PROPERTY
    QT_QML_IMPORT_PATH "${ZoinGallery_QML_IMPORT_PATH}"
  )
endif()

if(TARGET msgpack-cxx)
  target_link_libraries(f4-qt-host PRIVATE msgpack-cxx)
elseif(TARGET msgpack-cxx::msgpack-cxx)
  target_link_libraries(f4-qt-host PRIVATE msgpack-cxx::msgpack-cxx)
else()
  message(FATAL_ERROR "msgpack-cxx CMake target was not generated by Conan")
endif()

qt6_finalize_target(f4-qt-host)

install(TARGETS f4-qt-host RUNTIME DESTINATION bin)
install(FILES icons/lucide/LICENSE
  DESTINATION licenses
  RENAME Lucide-LICENSE.txt
)
if(USE_QWK)
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
else()
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
  endfunction()

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
    Qt6::Quick
    Qt6::Svg
    Qt6::Test
  )
  add_test(NAME F4IconProviderTest COMMAND F4IconProviderTests)
  set_tests_properties(F4IconProviderTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )
  if(APPLE OR WIN32)
    add_test(NAME F4IconProviderNativeSmoke
      COMMAND F4IconProviderTests nativePlatformFileIconSmoke)
    set_tests_properties(F4IconProviderNativeSmoke PROPERTIES
      ENVIRONMENT "QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
    )
  endif()

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
  add_test(NAME F4PanelSplitterTest COMMAND F4PanelSplitterTests)
  set_tests_properties(F4PanelSplitterTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  add_executable(F4GalleryBridgeTests
    tests/F4GalleryBridgeTests.cpp
    src/F4GalleryBridge.h
    src/F4GalleryBridge.cpp
    src/F4IconProvider.h
    src/F4IconProvider.cpp
  )
  target_include_directories(F4GalleryBridgeTests PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
  f4_add_icon_test_resources(F4GalleryBridgeTests)
  target_compile_definitions(F4GalleryBridgeTests PRIVATE
    F4_WITH_ZOINGALLERY=$<BOOL:${F4_WITH_ZOINGALLERY}>
  )
  target_link_libraries(F4GalleryBridgeTests PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::QuickControls2
    Qt6::Svg
    Qt6::Test
  )
  if(F4_WITH_ZOINGALLERY)
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
  endif()
  add_test(NAME F4GalleryBridgeTest COMMAND F4GalleryBridgeTests)
  set_tests_properties(F4GalleryBridgeTest PROPERTIES
    ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
  )

  if(F4_WITH_ZOINGALLERY)
    add_executable(F4GalleryPointerTests
      tests/F4GalleryPointerTests.cpp
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
    target_compile_definitions(F4GalleryPointerTests PRIVATE F4_WITH_ZOINGALLERY=1)
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
    if(TARGET msgpack-cxx)
      target_link_libraries(F4GalleryPointerTests PRIVATE msgpack-cxx)
    elseif(TARGET msgpack-cxx::msgpack-cxx)
      target_link_libraries(F4GalleryPointerTests PRIVATE msgpack-cxx::msgpack-cxx)
    endif()
    add_test(NAME F4GalleryPointerTest COMMAND F4GalleryPointerTests)
    set_tests_properties(F4GalleryPointerTest PROPERTIES
      ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QSG_RHI_BACKEND=software;QT_PLUGIN_PATH=${CMAKE_BINARY_DIR}/plugins"
    )
  endif()
endif()
