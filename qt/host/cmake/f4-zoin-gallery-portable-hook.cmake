if(PROJECT_NAME STREQUAL "ZoinGallery" AND
   COMMAND f4_strip_static_cpp_runtime_metadata)
  cmake_language(DEFER CALL f4_strip_static_cpp_runtime_metadata
    "${CMAKE_CURRENT_BINARY_DIR}")
endif()
