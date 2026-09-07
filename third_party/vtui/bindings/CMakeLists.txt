cmake_minimum_required(VERSION 3.14)
project(vtui_bindings C CXX)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 17)

# Build Go vtui-host executable
set(VTUI_HOST_BIN "${CMAKE_CURRENT_BINARY_DIR}/vtui-host")

add_custom_command(
    OUTPUT ${VTUI_HOST_BIN}
    COMMAND go build -o ${VTUI_HOST_BIN} ./cmd/vtui-host
    WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}/..
    COMMENT "Building Go vtui-host binary"
)

add_custom_target(vtui_host_target ALL DEPENDS ${VTUI_HOST_BIN})

add_subdirectory(c)
add_subdirectory(cpp)
