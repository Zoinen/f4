# Project Structure

    .
    ├── ansi_writer.go
    ├── ARCHITECTURE.md
    ├── ARCH_PROPOSALS.md
    ├── autocomplete.go
    ├── autocomplete_test.go
    ├── autolayout.go
    ├── AUTOLAYOUT.md
    ├── autolayout_test.go
    ├── automation_test.go
    ├── backend_info.go
    ├── backend_info_test.go
    ├── bar.go
    ├── bar_test.go
    ├── baseframe.go
    ├── baseframe_test.go
    ├── basewindow.go
    ├── basewindow_test.go
    ├── bidi.go
    ├── bidi_test.go
    ├── bindings
    │   ├── c
    │   │   ├── cabi
    │   │   │   └── main.go
    │   │   ├── CMakeLists.txt
    │   │   ├── examples
    │   │   │   └── hello.c
    │   │   ├── include
    │   │   │   ├── vtui_constants.h
    │   │   │   └── vtui.h
    │   │   ├── README.md
    │   │   └── src
    │   │       └── vtui.c
    │   ├── CMakeLists.txt
    │   ├── cpp
    │   │   ├── CMakeLists.txt
    │   │   ├── examples
    │   │   │   └── hello.cpp
    │   │   ├── include
    │   │   │   └── vtui.hpp
    │   │   └── README.md
    │   ├── lua
    │   │   ├── examples
    │   │   │   └── hello.lua
    │   │   ├── README.md
    │   │   ├── rockspec
    │   │   │   └── vtui-scm-1.rockspec
    │   │   ├── src
    │   │   │   └── vtui_lua.c
    │   │   ├── tests
    │   │   │   └── test_vtui.lua
    │   │   └── vtui.lua
    │   ├── node
    │   │   ├── examples
    │   │   │   ├── hello.js
    │   │   │   └── hello.ts
    │   │   ├── index.js
    │   │   ├── package.json
    │   │   ├── README.md
    │   │   ├── session.js
    │   │   ├── test
    │   │   │   └── test.js
    │   │   ├── ui.js
    │   │   └── vtui.d.ts
    │   ├── php
    │   │   ├── composer.json
    │   │   ├── examples
    │   │   │   └── hello.php
    │   │   ├── README.md
    │   │   ├── src
    │   │   │   └── Vtui.php
    │   │   └── tests
    │   │       └── test_vtui.php
    │   ├── python
    │   │   ├── examples
    │   │   │   ├── async_demo.py
    │   │   │   └── hello.py
    │   │   ├── README.md
    │   │   ├── tests
    │   │   │   ├── __pycache__
    │   │   │   │   └── test_vtui.cpython-312.pyc
    │   │   │   └── test_vtui.py
    │   │   └── vtui
    │   │       ├── async_session.py
    │   │       ├── __init__.py
    │   │       ├── _props.py
    │   │       ├── __pycache__
    │   │       │   ├── async_session.cpython-312.pyc
    │   │       │   ├── __init__.cpython-312.pyc
    │   │       │   ├── _props.cpython-312.pyc
    │   │       │   ├── session.cpython-312.pyc
    │   │       │   └── ui.cpython-312.pyc
    │   │       ├── session.py
    │   │       └── ui.py
    │   └── README.md
    ├── bindings_integration_test.go
    ├── bindings.md
    ├── box_runes.go
    ├── button.go
    ├── button_test.go
    ├── cellspan_test.go
    ├── checkbox.go
    ├── checkbox_test.go
    ├── checkgroup.go
    ├── clipboard.go
    ├── clipboard_gui_test.go
    ├── clipboard_test.go
    ├── clipboard_unix.go
    ├── clipboard_windows.go
    ├── clusters_test.go
    ├── cmd
    │   ├── fontprobe
    │   │   └── main.go
    │   ├── test-app
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-cast
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-dialog
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-gen
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-host
    │   │   └── main.go
    │   ├── vtui-lint
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-replay
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── vtui-wasm
    │   │   └── main.go
    │   └── vuic
    │       ├── main.go
    │       └── main_test.go
    ├── colors.go
    ├── colors_test.go
    ├── combobox_color_test.go
    ├── combobox.go
    ├── combobox_test.go
    ├── commands.go
    ├── common_dialogs.go
    ├── common_dialogs_test.go
    ├── crash_report.go
    ├── crash_report_pid_unix.go
    ├── crash_report_pid_windows.go
    ├── crash_report_stub.go
    ├── crash_report_test.go
    ├── debug.go
    ├── debug_test.go
    ├── desktop.go
    ├── desktop_test.go
    ├── dialog_test.go
    ├── docs
    │   ├── shell_scripting.md
    │   └── widgets.md
    ├── dragdrop.go
    ├── DRAGDROP.md
    ├── dragdrop_test.go
    ├── dynamictext.go
    ├── dynamictext_test.go
    ├── ebiten_dragdrop.go
    ├── ebiten_host.go
    ├── ebiten_keys.go
    ├── ebiten_renderer.go
    ├── ebiten_renderer_test.go
    ├── ebiten_stub.go
    ├── edit.go
    ├── edit_test.go
    ├── events.go
    ├── eventsink_test.go
    ├── examples
    │   └── shell
    │       └── demo.sh
    ├── factory.go
    ├── factory_test.go
    ├── far2l_extensions.go
    ├── far2l_extensions_test.go
    ├── filelist_update.sh
    ├── frame.go
    ├── framemanager.go
    ├── framemanager_hidebars_test.go
    ├── framemanager_test.go
    ├── fuzzy.go
    ├── fuzzy_test.go
    ├── .github
    │   └── workflows
    │       └── macos-test.yml
    ├── .gitignore
    ├── gogpu_customchar_test.go
    ├── gogpu_dnd.go
    ├── gogpu_dnd_test.go
    ├── gogpu_glyphgen_test.go
    ├── gogpu_glyph_table.go
    ├── gogpu_host.go
    ├── gogpu_host_test.go
    ├── gogpu_keys_test.go
    ├── gogpu_profile.go
    ├── gogpu_renderer.go
    ├── gogpu_renderer_test.go
    ├── gogpu_scroll_other.go
    ├── gogpu_scroll_windows.go
    ├── gogpu_stub.go
    ├── go.mod
    ├── go.sum
    ├── graphics_frame_test.go
    ├── graphics.go
    ├── graphics_image.go
    ├── graphics_kitty.go
    ├── graphics_kitty_test.go
    ├── GRAPHICS.md
    ├── graphics_native.go
    ├── graphics_native_test.go
    ├── graphics_probe.go
    ├── graphics_probe_test.go
    ├── graphics_probe_unix.go
    ├── graphics_probe_windows.go
    ├── graphics_scale.go
    ├── graphics_sixel_cursor_test.go
    ├── graphics_sixel.go
    ├── graphics_sixel_quality_test.go
    ├── graphics_sixel_tabrow_test.go
    ├── graphics_sixel_test.go
    ├── graphics_test.go
    ├── grid_nav.go
    ├── groupbox.go
    ├── group.go
    ├── group_test.go
    ├── grow_test.go
    ├── gui_api_fallback.go
    ├── gui_api.go
    ├── gui_boxdraw.go
    ├── gui_boxdraw_test.go
    ├── gui_font.go
    ├── gui_font_test.go
    ├── help_engine.go
    ├── help_engine_test.go
    ├── help_view.go
    ├── help_view_mouse_test.go
    ├── help_view_test.go
    ├── highlight.go
    ├── highlight_test.go
    ├── history_test.go
    ├── internal
    │   └── hideconsole
    │       ├── go.mod
    │       └── hideconsole.go
    ├── keybar.go
    ├── keybar_test.go
    ├── keys_common.go
    ├── keys_common_test.go
    ├── label.go
    ├── label_test.go
    ├── layout.go
    ├── LAYOUT.md
    ├── layout_test.go
    ├── layout_validator.go
    ├── layout_validator_test.go
    ├── LICENSE
    ├── listbox.go
    ├── listbox_test.go
    ├── localization.go
    ├── localization_test.go
    ├── lookup_test.go
    ├── menubar.go
    ├── menubar_test.go
    ├── multilineedit.go
    ├── multilineedit_test.go
    ├── OPTIMIZATIONS.md
    ├── painter.go
    ├── palette_batch_test.go
    ├── palette.go
    ├── palette_test.go
    ├── panic_bridge.go
    ├── panic_bridge_test.go
    ├── progressbar.go
    ├── progressbar_test.go
    ├── properties_gen.go
    ├── properties.go
    ├── properties_test.go
    ├── protocol.go
    ├── protocol_test.go
    ├── radiobutton.go
    ├── radiogroup.go
    ├── radiogroup_test.go
    ├── README.md
    ├── REVIEW.md
    ├── rowprovider_test.go
    ├── runewidth.go
    ├── runewidth_test.go
    ├── screenbuf.go
    ├── screenbuf_test.go
    ├── screenobject.go
    ├── screenobject_test.go
    ├── screenshot.png
    ├── scrollbar.go
    ├── scrollbar_test.go
    ├── scrollbar_widget_test.go
    ├── scrollview.go
    ├── scrollview_test.go
    ├── semantic.go
    ├── semantic_test.go
    ├── separator.go
    ├── session_test.go
    ├── shutdown_test.go
    ├── sizespec.go
    ├── spacer.go
    ├── standard_dialogs_layout_test.go
    ├── statusline.go
    ├── statusline_test.go
    ├── step_test.go
    ├── strings.go
    ├── symbols.go
    ├── sys_darwin.go
    ├── sys_unix.go
    ├── sys_windows.go
    ├── table_dialog.go
    ├── table_dialog_test.go
    ├── table.go
    ├── table_test.go
    ├── tasks.go
    ├── tasks_test.go
    ├── terminal_env.go
    ├── terminal_env_test.go
    ├── terminal_env_unix.go
    ├── terminal_env_windows.go
    ├── testdata
    │   ├── hello.golden.json
    │   └── hello.vui
    ├── testing.go
    ├── test_main_test.go
    ├── text.go
    ├── textseg.go
    ├── TEXTSEG.md
    ├── textseg_test.go
    ├── text_test.go
    ├── text_utils.go
    ├── text_utils_test.go
    ├── treeview.go
    ├── treeview_test.go
    ├── types.go
    ├── UI_TESTING.md
    ├── UNICODE_PLAN.md
    ├── UX_GUIDELINES.md
    ├── validator.go
    ├── validator_test.go
    ├── vmenu_cancel_test.go
    ├── vmenu.go
    ├── vmenu_test.go
    ├── vocabulary.json
    ├── vocabulary.schema.json
    ├── vreactive
    │   ├── animator.go
    │   ├── animator_test.go
    │   ├── bindings.go
    │   ├── bindings_test.go
    │   ├── computed.go
    │   ├── computed_test.go
    │   ├── easing.go
    │   ├── easing_test.go
    │   ├── property.go
    │   ├── property_test.go
    │   ├── README.md
    │   └── statemachine.go
    ├── vtext.go
    ├── vtext_test.go
    ├── vtui_test.go
    ├── vui_layout.go
    ├── vui_loader.go
    ├── vui.schema.json
    ├── vui_test.go
    ├── wayland_host.go
    ├── wayland_host_test.go
    ├── wayland_renderer.go
    ├── wayland_stub.go
    ├── wheel_scroll.go
    ├── wheel_scroll_test.go
    ├── WIDTH_NEGOTIATION.md
    ├── win32_console_common.go
    ├── win32_console_stub.go
    ├── win32_console_test.go
    ├── win32_console_windows.go
    ├── win32_gui_common.go
    ├── win32_gui_renderer.go
    ├── win32_gui_stub.go
    ├── win32_gui_test.go
    ├── win32_gui_windows.go
    ├── window.go
    ├── word_nav.go
    ├── WORDNAV.md
    ├── word_nav_test.go
    ├── workspace_altnumber_test.go
    ├── x11_host.go
    ├── x11_host_test.go
    ├── x11_keys_shared.go
    ├── x11_keys_test.go
    ├── x11_render_common.go
    ├── x11_renderer.go
    ├── x11_shm_fallback.go
    ├── x11_shm_unix.go
    ├── x11_xdnd.go
    ├── x11_xdnd_test.go
    ├── xlat.go
    ├── xlat_tables.go
    └── xlat_test.go

    48 directories, 360 files
