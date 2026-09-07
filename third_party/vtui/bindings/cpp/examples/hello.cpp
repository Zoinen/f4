#include <iostream>
#include "../include/vtui.hpp"

int main() {
    return vtui::run([](vtui::Ui& u) {
        auto d = u.dialog(" Hello vtui ", {.w = 40});
        auto name = u.edit("&Name:", "Type here...");
        if (u.button("&Ok")) {
            u.message(" Result ", "You typed:\n" + name);
        }
    });
}
