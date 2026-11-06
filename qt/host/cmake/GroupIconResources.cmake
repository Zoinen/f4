# Artwork aliases only: matching and group definitions belong to the shared catalog.
set(F4_ZOIN_GROUP_ICON_RENAMES
  bash=shell-scripts c=c-source claude=claude cpp=cpp-source css=stylesheets
  database=databases default=files docker=docker folder-up=parent-folder
  folder=folders font=fonts git=git go=go-source html=web-pages image=images
  javascript=javascript json=json markdown=markdown npm=npm python=python
  ruby=ruby rust=rust svg=vector-images swift=swift table=tables text=text-files
  typescript=typescript yml=yaml zip=archives react=react)

# group key = original Lucide source name (all eight stroke widths).
set(F4_LUCIDE_GROUP_ICON_NAMES_FILE
  "${CMAKE_CURRENT_LIST_DIR}/../icons/files/group-names.txt")
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS
  "${F4_LUCIDE_GROUP_ICON_NAMES_FILE}")
file(STRINGS "${F4_LUCIDE_GROUP_ICON_NAMES_FILE}" F4_LUCIDE_GROUP_ICON_ALIASES)

function(f4_prepared_file_icon_name source output)
  set(name "${source}")
  foreach(pair IN LISTS F4_LUCIDE_GROUP_ICON_ALIASES)
    string(REPLACE "=" ";" parts "${pair}")
    list(GET parts 0 group)
    list(GET parts 1 artwork)
    if(source STREQUAL artwork)
      set(name "${group}")
      break()
    endif()
  endforeach()
  set(${output} "${name}" PARENT_SCOPE)
endfunction()

function(f4_write_group_icon_qrc output root)
  string(REPLACE "&" "&amp;" root "${root}")
  string(REPLACE "<" "&lt;" root "${root}")
  string(REPLACE ">" "&gt;" root "${root}")
  set(content "<RCC><qresource prefix=\"/F4QtHost/icons\">\n")
  foreach(pair IN LISTS F4_ZOIN_GROUP_ICON_RENAMES)
    string(REPLACE "=" ";" parts "${pair}")
    list(GET parts 0 old)
    list(GET parts 1 group)
    if(NOT old STREQUAL group)
      string(APPEND content "<file alias=\"zoin/${old}.svg\">${root}/icons/zoin/${group}.svg</file>\n")
    endif()
  endforeach()
  foreach(pair IN LISTS F4_LUCIDE_GROUP_ICON_ALIASES)
    string(REPLACE "=" ";" parts "${pair}")
    list(GET parts 0 group)
    list(GET parts 1 artwork)
    if(NOT group STREQUAL artwork)
      foreach(step RANGE 1 8)
        string(APPEND content "<file alias=\"files/${step}/${artwork}.svg\">${root}/icons/files/${step}/${group}.svg</file>\n")
      endforeach()
    endif()
  endforeach()
  string(APPEND content "</qresource></RCC>\n")
  file(CONFIGURE OUTPUT "${output}" CONTENT "${content}" @ONLY)
endfunction()

function(f4_add_group_icon_resources target)
  set(qrc "${CMAKE_CURRENT_BINARY_DIR}/${target}_group_icon_aliases.qrc")
  get_filename_component(root "${CMAKE_CURRENT_FUNCTION_LIST_DIR}/.." ABSOLUTE)
  f4_write_group_icon_qrc("${qrc}" "${root}")
  qt6_add_resources(sources "${qrc}")
  # Gallery test targets belong to a subdirectory. Keep a target in the
  # command's own directory so Ninja can order cross-directory generation.
  add_custom_target(${target}_group_icon_aliases DEPENDS ${sources})
  add_dependencies(${target} ${target}_group_icon_aliases)
  target_sources(${target} PRIVATE ${sources})
endfunction()
