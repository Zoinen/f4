cmake_minimum_required(VERSION 3.23)

project(QtPixelGridProof LANGUAGES CXX)

if(POLICY CMP0091)
  cmake_policy(SET CMP0091 NEW)
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

find_package(Qt6 6.11 REQUIRED COMPONENTS Core Gui GuiPrivate Qml Quick QuickControls2 Test OpenGL ShaderTools)

qt_standard_project_setup(REQUIRES 6.11)

set(QT_PIXELGRID_PATCHED_QUICK_LIBRARY "" CACHE FILEPATH
  "Experiment-only Qt6Quick.lib with the Qt 6.11.1 source patch")

qt_add_executable(qt-pixelgrid-zoin
  src/main.cpp
  src/StaticQmlPluginImports.cpp
  src/QtQuickPixelGrid.h
)
set_property(TARGET qt-pixelgrid-zoin PROPERTY QT_QML_MODULE_NO_IMPORT_SCAN TRUE)
set_source_files_properties(qml/Main.qml PROPERTIES QT_RESOURCE_ALIAS Main.qml)
set_source_files_properties(qml/PerformanceScene.qml PROPERTIES QT_RESOURCE_ALIAS PerformanceScene.qml)
set_source_files_properties(qml/PixelBorderIcon.qml PROPERTIES QT_RESOURCE_ALIAS PixelBorderIcon.qml)
set_source_files_properties(qml/RasterDiagnosticIcons.qml PROPERTIES QT_RESOURCE_ALIAS RasterDiagnosticIcons.qml)
set_source_files_properties(qml/ShaderDiagnosticScene.qml PROPERTIES QT_RESOURCE_ALIAS ShaderDiagnosticScene.qml)
qt_add_qml_module(qt-pixelgrid-zoin
  URI QtPixelGrid.Proof
  VERSION 1.0
  NO_PLUGIN
  QML_FILES
    qml/Main.qml
    qml/PerformanceScene.qml
    qml/PixelBorderIcon.qml
    qml/RasterDiagnosticIcons.qml
    qml/ShaderDiagnosticScene.qml
  SOURCES
    src/viewer/ViewerResampleEffect.h
    src/viewer/ViewerResampleEffect.cpp
)
target_link_libraries(qt-pixelgrid-zoin PRIVATE
  Qt6::Core Qt6::Gui Qt6::GuiPrivate Qt6::Qml Qt6::Quick Qt6::QuickControls2 Qt6::Test Qt6::OpenGL
)
target_include_directories(qt-pixelgrid-zoin PRIVATE src/viewer)
if(WIN32)
  set(_pixelgrid_shader_targets NOGLSL)
else()
  set(_pixelgrid_shader_targets GLSL "150,300es")
endif()
qt_add_shaders(qt-pixelgrid-zoin pixelgrid_diagnostic_shaders
  BATCHABLE
  ${_pixelgrid_shader_targets}
  PREFIX "/ZoinGallery"
  FILES
    resources/gallery_pixel_pattern.frag
    resources/pixelgrid_solid.frag
    resources/pixelgrid_passthrough.vert
    resources/viewer_resample.vert
    resources/viewer_resample.frag
)
target_compile_definitions(qt-pixelgrid-zoin PRIVATE QT_PIXELGRID_SOURCE_DIR="${CMAKE_CURRENT_SOURCE_DIR}")

if(QT_PIXELGRID_PATCHED_QUICK_LIBRARY)
  if(NOT EXISTS "${QT_PIXELGRID_PATCHED_QUICK_LIBRARY}")
    message(FATAL_ERROR "Patched Qt Quick archive does not exist: ${QT_PIXELGRID_PATCHED_QUICK_LIBRARY}")
  endif()
  add_library(Qt6QuickPixelGrid STATIC IMPORTED GLOBAL)
  set_target_properties(Qt6QuickPixelGrid PROPERTIES
    IMPORTED_LOCATION "${QT_PIXELGRID_PATCHED_QUICK_LIBRARY}")
  get_target_property(_quick_links Qt6::Quick INTERFACE_LINK_LIBRARIES)
  if(NOT _quick_links)
    message(FATAL_ERROR "Qt6::Quick has no visible Conan link facade")
  endif()
  list(TRANSFORM _quick_links REPLACE
    "CONAN_LIB::qt_Qt6_Quick[^,>]*" Qt6QuickPixelGrid)
  set_property(TARGET Qt6::Quick PROPERTY INTERFACE_LINK_LIBRARIES "${_quick_links}")
  target_compile_definitions(qt-pixelgrid-zoin PRIVATE QT_PIXELGRID_PATCHED_QT=1)
endif()

# Conan ships Qt's static QML plugins, but its component facade deliberately
# does not load their preserved native Qt6*Targets.cmake exports for consumers.
# Import the small module/plugin graph needed by this proof, matching the
# repository's F4 host setup without changing the Conan package itself.
set(_pixelgrid_qt_package "${qt_PACKAGE_FOLDER_RELEASE}")
set(_pixelgrid_qml_plugin_dir
  "${_pixelgrid_qt_package}/lib/cmake/Qt6Qml/QmlPlugins")
if(NOT EXISTS "${_pixelgrid_qml_plugin_dir}")
  message(FATAL_ERROR "Qt static QML plugin exports are missing: ${_pixelgrid_qml_plugin_dir}")
endif()

if(NOT TARGET Qt6::PlatformCommonInternal)
  add_library(Qt6::PlatformCommonInternal INTERFACE IMPORTED GLOBAL)
  set_target_properties(Qt6::PlatformCommonInternal PROPERTIES
    INTERFACE_LINK_LIBRARIES Qt6::Platform)
endif()
foreach(_platform_target IN ITEMS PlatformModuleInternal PlatformPluginInternal)
  if(NOT TARGET Qt6::${_platform_target})
    add_library(Qt6::${_platform_target} INTERFACE IMPORTED GLOBAL)
    set_target_properties(Qt6::${_platform_target} PROPERTIES
      INTERFACE_LINK_LIBRARIES Qt6::PlatformCommonInternal)
  endif()
endforeach()

function(pixelgrid_import_qml_backing_module module_name)
  if(TARGET Qt6::${module_name})
    return()
  endif()
  find_library(_pixelgrid_${module_name}_archive
    NAMES Qt6${module_name}
    PATHS "${_pixelgrid_qt_package}/lib"
    NO_DEFAULT_PATH REQUIRED)
  add_library(Qt6::${module_name} STATIC IMPORTED GLOBAL)
  set_target_properties(Qt6::${module_name} PROPERTIES
    IMPORTED_LOCATION "${_pixelgrid_${module_name}_archive}"
    INTERFACE_LINK_LIBRARIES "${ARGN}")
endfunction()

pixelgrid_import_qml_backing_module(QmlMeta
  Qt6::Core Qt6::Qml Qt6::QmlModels Qt6::QmlWorkerScript)
pixelgrid_import_qml_backing_module(QuickControls2Impl
  Qt6::Core Qt6::Gui Qt6::Quick Qt6::Qml Qt6::QuickTemplates2)
pixelgrid_import_qml_backing_module(QuickControls2BasicStyleImpl
  Qt6::Core Qt6::Gui Qt6::Qml Qt6::QuickControls2Impl Qt6::Quick
  Qt6::QuickTemplates2)
pixelgrid_import_qml_backing_module(QuickControls2Basic
  Qt6::Core Qt6::Gui Qt6::Qml Qt6::QuickControls2BasicStyleImpl
  Qt6::QuickControls2 Qt6::Quick Qt6::QuickTemplates2)
pixelgrid_import_qml_backing_module(QuickLayouts
  Qt6::Core Qt6::Qml Qt6::Gui Qt6::Quick)

set(_pixelgrid_qml_plugins
  qmlplugin modelsplugin workerscriptplugin qtquick2plugin quickwindow
  qquicklayoutsplugin qtquicktemplates2plugin qtquickcontrols2implplugin
  qtquickcontrols2basicstyleimplplugin qtquickcontrols2basicstyleplugin
  qtquickcontrols2plugin)
foreach(_plugin IN LISTS _pixelgrid_qml_plugins)
  set(_target_file "${_pixelgrid_qml_plugin_dir}/Qt6${_plugin}Targets.cmake")
  set(_info_file "${_pixelgrid_qml_plugin_dir}/Qt6${_plugin}AdditionalTargetInfo.cmake")
  if(NOT TARGET Qt6::${_plugin} AND EXISTS "${_target_file}")
    set(CMAKE_FIND_PACKAGE_NAME PixelGridStaticQmlPlugin)
    include("${_target_file}")
    if(EXISTS "${_info_file}")
      include("${_info_file}")
    endif()
  endif()
endforeach()

target_link_libraries(qt-pixelgrid-zoin PRIVATE
  Qt6::qmlplugin Qt6::modelsplugin Qt6::workerscriptplugin
  Qt6::qtquick2plugin Qt6::quickwindow Qt6::qquicklayoutsplugin
  Qt6::qtquicktemplates2plugin Qt6::qtquickcontrols2implplugin
  Qt6::qtquickcontrols2basicstyleimplplugin
  Qt6::qtquickcontrols2basicstyleplugin Qt6::qtquickcontrols2plugin
  Qt6::QuickControls2Basic Qt6::QuickControls2BasicStyleImpl
  Qt6::QuickControls2Impl Qt6::QuickLayouts
)
if(NOT TARGET Qt6::QuickControls2Basic_resources_1)
  set_property(TARGET qt-pixelgrid-zoin APPEND PROPERTY LINK_LIBRARY_OVERRIDE
    "WHOLE_ARCHIVE,Qt6::QuickControls2Basic")
endif()

if(WIN32)
  target_link_libraries(qt-pixelgrid-zoin PRIVATE Qt6::QWindowsIntegrationPlugin)
  target_sources(qt-pixelgrid-zoin PRIVATE src/StaticPlatformPluginImports.cpp)
endif()
if(TARGET Qt6::QOffscreenIntegrationPlugin)
  target_link_libraries(qt-pixelgrid-zoin PRIVATE Qt6::QOffscreenIntegrationPlugin)
  target_sources(qt-pixelgrid-zoin PRIVATE src/StaticOffscreenPluginImport.cpp)
endif()

if(MSVC)
  # Match the Conan-built Qt 6.11.1 package: /MT Release.
  set_property(TARGET qt-pixelgrid-zoin PROPERTY MSVC_RUNTIME_LIBRARY
    "MultiThreaded$<$<CONFIG:Debug>:Debug>")
  # Retain evidence that the linker selected the experiment archive members.
  target_link_options(qt-pixelgrid-zoin PRIVATE
    "/MAP:${CMAKE_CURRENT_BINARY_DIR}/qt-pixelgrid-zoin.map")
endif()

enable_testing()
set(_pixelgrid_scale_cases
  "1.0|0.5714285714285714"
  "1.25|0.7142857142857143"
  "1.5|0.8571428571428571"
  "2.0|1.1428571428571428"
)
foreach(_scale_case IN LISTS _pixelgrid_scale_cases)
  string(REPLACE "|" ";" _scale_values "${_scale_case}")
  list(GET _scale_values 0 _scale)
  list(GET _scale_values 1 _scale_factor)
  string(REPLACE "." "_" _scale_name "${_scale}")
  add_test(NAME pixelgrid_geometry_${_scale_name}
    COMMAND qt-pixelgrid-zoin --check --geometry-only)
  set_tests_properties(pixelgrid_geometry_${_scale_name} PROPERTIES
    TIMEOUT 20
    ENVIRONMENT "QT_QUICK_CONTROLS_STYLE=Basic;QT_SCALE_FACTOR=${_scale_factor};QT_PIXELGRID_EXPECT_DPR=${_scale}")
endforeach()
