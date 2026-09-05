#include <stdio.h>
#include "../include/vtui.h"

static void ui(vtui_ui *u) {
    static char name[128] = "Type here...";
    vtui_dialog(u, " Hello vtui ", 40);
      vtui_edit(u, "&Name:", name, sizeof(name));
      if (vtui_button(u, "&Ok")) {
          vtui_message(u, " Result ", name);
      }
    vtui_end(u);
}

int main(void) {
    return vtui_run(ui);
}
