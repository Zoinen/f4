# Project Structure
Last updated: 2026-08-01 17:38:36

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
├── colors.go
├── colors_test.go
├── command_line.go
├── command_line_test.go
├── commands.go
├── config.go
├── config_test.go
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
├── gui_unix.go
├── gui_windows.go
├── help.go
├── help.hlf
├── help_test.go
├── highlight_files.go
├── highlight_files_test.go
├── HIGHLIGHTING.md
├── history_provider.go
├── history_provider_test.go
├── info_panel.go
├── info_panel_test.go
├── ini.go
├── ini_test.go
├── input_translation.go
├── input_translation_test.go
├── issue149_test.go
├── issue54_test.go
├── lang.go
├── lang_test.go
├── LICENSE
├── LUA.md
├── macro.go
├── macro_test.go
├── main.go
├── mem_info.go
├── mem_info_linux.go
├── mem_info_other.go
├── mem_info_windows.go
├── panels_frame.go
├── panels_frame_test.go
├── piecetable
│   ├── lineindex.go
│   ├── lineindex_test.go
│   ├── piecetable.go
│   └── piecetable_test.go
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
├── plugins.go
├── PLUGINS.md
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
└── viewer_view_test.go

19 directories, 243 files
```
