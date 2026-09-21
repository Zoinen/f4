# The macOS app bundle is copied from the build tree by the custom post-build
# command.  That copy retains CMake's build-tree RPATH, while the bare host
# installed by install(TARGETS) receives INSTALL_RPATH.  Repair the bundle
# copy after it has been installed into the relocatable sidecar tree.

set(F4_MACOS_APP_EXECUTABLE
  "$ENV{DESTDIR}${CMAKE_INSTALL_PREFIX}/bin/f4-qt-host.app/Contents/MacOS/f4-qt-host")

if(NOT EXISTS "${F4_MACOS_APP_EXECUTABLE}")
  message(FATAL_ERROR
    "Installed macOS Qt app executable is missing: ${F4_MACOS_APP_EXECUTABLE}")
endif()

execute_process(
  COMMAND otool -l "${F4_MACOS_APP_EXECUTABLE}"
  RESULT_VARIABLE F4_OTOOL_RESULT
  OUTPUT_VARIABLE F4_OTOOL_OUTPUT
  ERROR_VARIABLE F4_OTOOL_ERROR
)
if(NOT F4_OTOOL_RESULT EQUAL 0)
  message(FATAL_ERROR
    "otool could not inspect the installed macOS Qt app executable: ${F4_OTOOL_ERROR}")
endif()

# Delete every build-tree RPATH rather than trying to predict Conan's package
# revisions.  The app executable lives four directories below the integrated
# tree root: bin/f4-qt-host.app/Contents/MacOS/f4-qt-host.
string(REPLACE "\n" ";" F4_OTOOL_LINES "${F4_OTOOL_OUTPUT}")
foreach(F4_OTOOL_LINE IN LISTS F4_OTOOL_LINES)
  string(STRIP "${F4_OTOOL_LINE}" F4_OTOOL_LINE)
  if(F4_OTOOL_LINE MATCHES "^path (.*) \\(offset [0-9]+\\)$")
    set(F4_MACOS_BUILD_RPATH "${CMAKE_MATCH_1}")
    execute_process(
      COMMAND install_name_tool -delete_rpath "${F4_MACOS_BUILD_RPATH}"
        "${F4_MACOS_APP_EXECUTABLE}"
      RESULT_VARIABLE F4_DELETE_RPATH_RESULT
      ERROR_VARIABLE F4_DELETE_RPATH_ERROR
    )
    if(NOT F4_DELETE_RPATH_RESULT EQUAL 0)
      message(FATAL_ERROR
        "Could not remove macOS app RPATH ${F4_MACOS_BUILD_RPATH}: ${F4_DELETE_RPATH_ERROR}")
    endif()
  endif()
endforeach()

execute_process(
  COMMAND install_name_tool -add_rpath "@loader_path/../../../../lib"
    "${F4_MACOS_APP_EXECUTABLE}"
  RESULT_VARIABLE F4_ADD_RPATH_RESULT
  ERROR_VARIABLE F4_ADD_RPATH_ERROR
)
if(NOT F4_ADD_RPATH_RESULT EQUAL 0)
  message(FATAL_ERROR
    "Could not add the relocatable macOS app RPATH: ${F4_ADD_RPATH_ERROR}")
endif()
