# C Bindings for vtui

This directory provides native C language bindings and an immediate-mode facade for `vtui`.

## Prerequisites

- Go compiler (1.26+)
- CMake (3.14+) or C compiler (GCC, Clang, MSVC)

## Building with CMake (from `bindings/c`)

```bash
mkdir -p build && cd build
cmake ..
cmake --build .
./hello_c
```

## Building manually with GCC / Clang (from `bindings/c`)

```bash
# 1. Compile the vtui-host binary
go build -o build/vtui-host ../../cmd/vtui-host

# 2. Compile and link the C application
gcc -Iinclude src/vtui.c examples/hello.c -o build/hello_c

# 3. Run
cd build && ./hello_c
```
