# C++ Bindings for vtui

Modern, header-only C++17 wrapper for `vtui`.

## Usage Example

```cpp
#include <vtui.hpp>

int main() {
    return vtui::run([](vtui::Ui& u) {
        auto d = u.dialog(" Hello vtui ", {.w = 40});
        auto name = u.edit("&Name:", "Type here...");
        if (u.button("&Ok")) {
            u.message(" Result ", "You typed:\n" + name);
        }
    });
}
```

## Building with CMake (from `bindings/cpp`)

```bash
mkdir -p build && cd build
cmake ..
cmake --build .
./hello_cpp
```

## Building manually with G++ / Clang++ (from `bindings/cpp`)

```bash
# 1. Compile the vtui-host binary
go build -o build/vtui-host ../../cmd/vtui-host

# 2. Compile and link the C++ application
g++ -std=c++17 -I../c/include -Iinclude ../c/src/vtui.c examples/hello.cpp -o build/hello_cpp

# 3. Run
cd build && ./hello_cpp
```
