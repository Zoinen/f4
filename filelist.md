# Project Structure
Last updated: 2026-08-04 07:43:17

```text
.
├── actions.go
├── actions_test.go
├── ansi_parser.go
├── ansi_parser_test.go
├── api.go
├── api_test.go
├── archive_index_fallback.go
├── archive_index.go
├── archive_index_test.go
├── arkanoid.go
├── arkanoid_test.go
├── assets
│   └── icon
│       ├── f4-16.svg
│       ├── f4-24.svg
│       ├── f4-30.svg
│       ├── f4-32.svg
│       ├── f4-36.svg
│       ├── f4-42.svg
│       ├── f4.svg
│       ├── generated
│       │   ├── f4-1024.png
│       │   ├── f4-128.png
│       │   ├── f4-16.png
│       │   ├── f4-24.png
│       │   ├── f4-256.png
│       │   ├── f4-28.png
│       │   ├── f4-30.png
│       │   ├── f4-32.png
│       │   ├── f4-36.png
│       │   ├── f4-42.png
│       │   ├── f4-48.png
│       │   ├── f4-512.png
│       │   ├── f4-56.png
│       │   ├── f4-64.png
│       │   ├── f4.icns
│       │   └── f4.ico
│       └── README.md
├── async_buffer.go
├── async_buffer_test.go
├── attributes_dialog.go
├── attributes_dialog_unix.go
├── attributes_dialog_windows.go
├── attributes_test.go
├── bookmarks_dialog.go
├── bookmarks_dialog_test.go
├── bookmarks.go
├── bookmarks_test.go
├── child_env.go
├── child_env_test.go
├── colorer_downloader.go
├── colorer_hrc.go
├── colorer_hrc_test.go
├── colorer_hrd.go
├── colorer_hrd_test.go
├── colorer_plugin.go
├── colorer_plugin_test.go
├── colorer_settings.go
├── colorer_settings_test.go
├── colors.go
├── colors_test.go
├── command_line.go
├── command_line_test.go
├── commands.go
├── config.go
├── config_test.go
├── cpu_info_darwin.go
├── cpu_info.go
├── cpu_info_linux.go
├── cpu_info_other.go
├── cpu_info_windows.go
├── detach_unix.go
├── detach_windows.go
├── drives_unix.go
├── drives_windows.go
├── editor_features_test.go
├── editor_view_ads_test.go
├── editor_view.go
├── editor_view_test.go
├── extui_host.go
├── extui_test.go
├── far2l_auth.go
├── farcolor_exp.go
├── farcolor_test.go
├── farmenu_file.go
├── farmenu_file_test.go
├── ffibridge
│   ├── bridge.go
│   ├── bridge_test.go
│   ├── callback.go
│   ├── convert.go
│   ├── convert_test.go
│   ├── kind.go
│   ├── libc.go
│   ├── memory.go
│   ├── memory_test.go
│   ├── signature.go
│   ├── signature_test.go
│   ├── sys_ffi.go
│   └── sys_stub.go
├── FFI.md
├── filelist_update.sh
├── file_op_dialog.go
├── file_op_dialog_test.go
├── file_ops.go
├── file_ops_test.go
├── file_op_tracker.go
├── file_op_tracker_test.go
├── file_panel.go
├── file_panel_test.go
├── file_state.go
├── file_state_test.go
├── find_file.go
├── find_file_test.go
├── FISH+.md
├── fs_info_darwin.go
├── fs_info.go
├── fs_info_linux.go
├── fs_info_other.go
├── fs_info_windows.go
├── .github
│   └── workflows
│       └── build.yml
├── .gitignore
├── go.mod
├── go.sum
├── gpu_info_darwin.go
├── gpu_info.go
├── gpu_info_linux.go
├── gpu_info_other.go
├── gpu_info_windows.go
├── gui_unix.go
├── gui_windows.go
├── help
│   ├── en.hlf
│   └── ru.hlf
├── help.go
├── help_test.go
├── highlight_files.go
├── highlight_files_test.go
├── HIGHLIGHTING.md
├── history_provider.go
├── history_provider_test.go
├── I18N.md
├── image_bmp.go
├── image_decode.go
├── image_decode_test.go
├── image_formats_test.go
├── image_gallery.go
├── image_gallery_test.go
├── image_pipeline.go
├── image_pipeline_test.go
├── image_preview.go
├── image_preview_test.go
├── image_qoi.go
├── image_slideshow.go
├── image_slideshow_test.go
├── image_transform.go
├── image_transform_test.go
├── image_view.go
├── image_view_orient_test.go
├── image_view_overlay_test.go
├── image_view_test.go
├── info_panel.go
├── info_panel_test.go
├── ini.go
├── ini_test.go
├── input_translation.go
├── input_translation_test.go
├── issue149_test.go
├── issue54_test.go
├── kitty_graphics.go
├── kitty_graphics_test.go
├── kitty_metrics_test.go
├── kitty_placements.go
├── kitty_placements_test.go
├── lang
│   ├── en.lng
│   └── ru.lng
├── lang.go
├── lang_test.go
├── LICENSE
├── LUA.md
├── luaplug
│   ├── convert.go
│   ├── convert_test.go
│   ├── f4rpc.go
│   ├── ffi.go
│   ├── ffi_test.go
│   ├── goid.go
│   ├── luastate_test.go
│   ├── runtime.go
│   ├── runtime_test.go
│   └── sandbox.go
├── lua_plugin.go
├── lua_plugin_test.go
├── macro_export.go
├── macro_export_test.go
├── macro.go
├── macro_host.go
├── macro_lua_api.go
├── macro_lua.go
├── macro_lua_test.go
├── MACROS.md
├── macro_test.go
├── main.go
├── mem_info.go
├── mem_info_linux.go
├── mem_info_other.go
├── mem_info_windows.go
├── packaging
│   ├── linux
│   │   └── f4.desktop
│   └── macos
│       └── Info.plist
├── panels_frame.go
├── panels_frame_test.go
├── piecetable
│   ├── lineindex.go
│   ├── lineindex_test.go
│   ├── piecetable.go
│   └── piecetable_test.go
├── plughost_ffi.go
├── plughost_ffi_test.go
├── plughost.go
├── plugin_identity_test.go
├── plugin_permissions.go
├── plugin_permissions_test.go
├── plugin_permissions_ui.go
├── plugin_permissions_ui_test.go
├── PLUGIN_PLAN.md
├── plugins
│   ├── archive
│   │   ├── archive.go
│   │   ├── archive_test.go
│   │   ├── provider.go
│   │   ├── provider_test.go
│   │   ├── repro_test.go
│   │   ├── vfs.go
│   │   ├── vfs_nested_test.go
│   │   ├── vfs_test.go
│   │   └── zip_encoding.go
│   ├── chroma
│   │   ├── chroma.go
│   │   └── chroma_test.go
│   ├── dummy_internal
│   │   ├── dummy_internal.go
│   │   └── dummy_internal_test.go
│   ├── dummy_lua
│   │   ├── plugin.lua
│   │   └── README.md
│   ├── dummy_rpc
│   │   └── main.go
│   └── netfox
│       ├── crypto.go
│       ├── crypto_test.go
│       ├── dialog.go
│       ├── dialog_test.go
│       ├── ftp_vfs.go
│       ├── netfox.go
│       ├── netfox_test.go
│       ├── registry.go
│       ├── sftp_vfs.go
│       ├── ssh_pty.go
│       ├── vfs_abs_test.go
│       └── vfs.go
├── plugin_scaffold.go
├── plugin_scaffold_test.go
├── plugins.go
├── PLUGINS.md
├── plugring
│   ├── hello_plugring.lua
│   └── index.yaml
├── plugring.go
├── PLUGRING.md
├── plugring_meta.go
├── plugring_meta_test.go
├── plugring_policy_test.go
├── plugring_rows_test.go
├── plugring_test.go
├── plugring_ui.go
├── plugring_ui_test.go
├── portable_test.go
├── pty_bsd.go
├── pty_darwin.go
├── pty_interface.go
├── pty_ptm.go
├── pty_solaris.go
├── pty_test.go
├── pty_unix.go
├── pty_windows.go
├── queue_manager.go
├── queue_manager_test.go
├── quick_view_panel.go
├── quick_view_panel_test.go
├── README.md
├── rpc_lua_test.go
├── rpc_plugin.go
├── rpc_plugin_test.go
├── rpc_vfs.go
├── rpc_vfs_test.go
├── rsrc_windows_amd64.syso
├── rsrc_windows_arm64.syso
├── screenshot.png
├── sdk
│   ├── extui
│   │   ├── model.go
│   │   └── model_test.go
│   ├── f4plugin
│   │   └── plugin.go
│   ├── f4rpc
│   │   ├── mux.go
│   │   └── mux_test.go
│   └── lua
│       └── f4rpc.lua
├── semantic.go
├── semantic_test.go
├── session_unix.go
├── session_unix_test.go
├── session_windows.go
├── shell_integration_test.go
├── solaris_pty_alloc_test.go
├── solaris_pty_backend_test.go
├── solaris_pty.go
├── solaris_streams.go
├── solaris_streams_mock_test.go
├── solaris_streams_test.go
├── style.go
├── styles
│   ├── classic.ini
│   └── modern.ini
├── style_test.go
├── terminal_log_vfs.go
├── terminal_log_vfs_test.go
├── TERMINAL.md
├── terminal_view.go
├── terminal_view_test.go
├── test_main_test.go
├── test_plugins.sh
├── test_resurrect.sh
├── textlayout
│   ├── wrap.go
│   └── wrap_test.go
├── title.go
├── title_test.go
├── title_unix.go
├── title_windows.go
├── tools
│   └── icons
│       ├── go.mod
│       ├── go.sum
│       ├── main.go
│       ├── main_test.go
│       └── third_party
│           └── oksvg
│               ├── definitions.go
│               ├── draw.go
│               ├── .gitignore
│               ├── go.mod
│               ├── icon_cursor.go
│               ├── LICENSE
│               ├── path_cursor.go
│               ├── path_style.go
│               ├── public.go
│               ├── README.md
│               ├── svg_icon.go
│               ├── svg_path.go
│               └── utils.go
├── top_bar.go
├── top_bar_test.go
├── translate_kitty.go
├── translate_kitty_test.go
├── updater.go
├── updater_test.go
├── user_menu.go
├── user_menu_ini.go
├── user_menu_ini_test.go
├── user_menu_subst.go
├── user_menu_subst_test.go
├── user_menu_ui.go
├── user_menu_ui_test.go
├── UX_GUIDELINES.md
├── vfs
│   ├── bulk_copy_test.go
│   ├── codepages.go
│   ├── codepages_test.go
│   ├── codepages_unix.go
│   ├── codepages_windows.go
│   ├── hidden_unix.go
│   ├── hidden_windows.go
│   ├── isabs_test.go
│   ├── lock_manager_test.go
│   ├── null_vfs.go
│   ├── null_vfs_test.go
│   ├── os_vfs_dot_test.go
│   ├── os_vfs.go
│   ├── os_vfs_junction_stub.go
│   ├── os_vfs_physical_other.go
│   ├── os_vfs_physical_test.go
│   ├── os_vfs_physical_unix.go
│   ├── os_vfs_physical_windows.go
│   ├── os_vfs_platform_unix.go
│   ├── os_vfs_platform_windows.go
│   ├── os_vfs_posix_atimespec.go
│   ├── os_vfs_posix_atim.go
│   ├── os_vfs_symlink_test.go
│   ├── os_vfs_test.go
│   ├── os_vfs_unix_test.go
│   ├── os_vfs_windows.go
│   ├── os_vfs_windows_test.go
│   ├── privileges_windows.go
│   ├── scanner.go
│   ├── scanner_test.go
│   ├── sudo_askpass_unix.go
│   ├── sudo_askpass_windows.go
│   ├── sudo_client.go
│   ├── sudo_dispatcher_unix.go
│   ├── sudo_dispatcher_windows.go
│   ├── sudo_ipc_unix.go
│   ├── sudo_ipc_windows.go
│   ├── sudo_msg.go
│   ├── sudo_test.go
│   ├── utils.go
│   ├── utils_test.go
│   └── vfs.go
├── VFS.md
├── viewer_backend.go
├── viewer_backend_test.go
├── viewer_view.go
├── viewer_view_test.go
├── wasm_plugin.go
├── wasm_plugin_test.go
├── window_icon_windows.go
└── window_icon_windows_test.go

34 directories, 401 files
```
