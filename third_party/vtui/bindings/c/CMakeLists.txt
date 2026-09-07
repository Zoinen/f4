cmake_minimum_required(VERSION 3.14)
project(vtui_c C)

set(CMAKE_C_STANDARD 11)

if(NOT TARGET vtui_host_target)
    set(VTUI_HOST_BIN "${CMAKE_CURRENT_BINARY_DIR}/vtui-host")
    add_custom_command(
        OUTPUT ${VTUI_HOST_BIN}
        COMMAND go build -o ${VTUI_HOST_BIN} ./cmd/vtui-host
        WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}/../..
        COMMENT "Building Go vtui-host binary"
    )
    add_custom_target(vtui_host_target ALL DEPENDS ${VTUI_HOST_BIN})
endif()

add_library(vtui_c STATIC
    src/vtui.c
)

target_include_directories(vtui_c PUBLIC
    include
)

add_executable(hello_c
    examples/hello.c
)

target_link_libraries(hello_c PRIVATE
    vtui_c
)

add_dependencies(hello_c vtui_host_target)
