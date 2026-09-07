local vtui = {}

-- --- Embedded Pure-Lua JSON Codec ---
local json = {}

function json.encode(val)
    local t = type(val)
    if t == "nil" then
        return "null"
    elseif t == "boolean" then
        return val and "true" or "false"
    elseif t == "number" then
        return tostring(val)
    elseif t == "string" then
        local s = val:gsub('\\', '\\\\'):gsub('"', '\\"'):gsub('\n', '\\n'):gsub('\r', '\\r'):gsub('\t', '\\t')
        return '"' .. s .. '"'
    elseif t == "table" then
        local is_array = true
        local n = 0
        for k, _ in pairs(val) do
            n = n + 1
            if type(k) ~= "number" or k ~= n then
                is_array = false
                break
            end
        end
        if is_array then
            local parts = {}
            for i = 1, #val do
                parts[i] = json.encode(val[i])
            end
            return "[" .. table.concat(parts, ",") .. "]"
        else
            local parts = {}
            for k, v in pairs(val) do
                table.insert(parts, json.encode(tostring(k)) .. ":" .. json.encode(v))
            end
            return "{" .. table.concat(parts, ",") .. "}"
        end
    end
    return "null"
end

function json.decode(str)
    local pos = 1
    local len = #str

    local function skip_ws()
        while pos <= len and str:sub(pos, pos):match("%s") do
            pos = pos + 1
        end
    end

    local parse_val

    local function parse_str()
        pos = pos + 1 -- skip opening quote
        local parts = {}
        while pos <= len do
            local c = str:sub(pos, pos)
            if c == '"' then
                pos = pos + 1
                return table.concat(parts)
            elseif c == '\\' then
                pos = pos + 1
                local esc = str:sub(pos, pos)
                if esc == 'n' then table.insert(parts, '\n')
                elseif esc == 'r' then table.insert(parts, '\r')
                elseif esc == 't' then table.insert(parts, '\t')
                elseif esc == '"' then table.insert(parts, '"')
                elseif esc == '\\' then table.insert(parts, '\\')
                elseif esc == '/' then table.insert(parts, '/')
                else table.insert(parts, esc) end
                pos = pos + 1
            else
                table.insert(parts, c)
                pos = pos + 1
            end
        end
        error("unterminated string in JSON")
    end

    local function parse_num()
        local s, e = str:find("^%-?%d+%.?%d*[eE]?[%+%-]?%d*", pos)
        if s then
            local n = tonumber(str:sub(s, e))
            pos = e + 1
            return n
        end
        error("invalid number in JSON at pos " .. pos)
    end

    local function parse_arr()
        pos = pos + 1 -- skip '['
        local arr = {}
        skip_ws()
        if str:sub(pos, pos) == ']' then
            pos = pos + 1
            return arr
        end
        while pos <= len do
            table.insert(arr, parse_val())
            skip_ws()
            local c = str:sub(pos, pos)
            if c == ']' then
                pos = pos + 1
                return arr
            elseif c == ',' then
                pos = pos + 1
                skip_ws()
            else
                error("expected ']' or ',' in array at pos " .. pos)
            end
        end
        error("unterminated array in JSON")
    end

    local function parse_obj()
        pos = pos + 1 -- skip '{'
        local obj = {}
        skip_ws()
        if str:sub(pos, pos) == '}' then
            pos = pos + 1
            return obj
        end
        while pos <= len do
            skip_ws()
            if str:sub(pos, pos) ~= '"' then
                error("expected string key in object at pos " .. pos)
            end
            local k = parse_str()
            skip_ws()
            if str:sub(pos, pos) ~= ':' then
                error("expected ':' after key in object at pos " .. pos)
            end
            pos = pos + 1
            obj[k] = parse_val()
            skip_ws()
            local c = str:sub(pos, pos)
            if c == '}' then
                pos = pos + 1
                return obj
            elseif c == ',' then
                pos = pos + 1
            else
                error("expected '}' or ',' in object at pos " .. pos)
            end
        end
        error("unterminated object in JSON")
    end

    parse_val = function()
        skip_ws()
        if pos > len then return nil end
        local c = str:sub(pos, pos)
        if c == '"' then
            return parse_str()
        elseif c == '{' then
            return parse_obj()
        elseif c == '[' then
            return parse_arr()
        elseif c == 't' and str:sub(pos, pos+3) == 'true' then
            pos = pos + 4
            return true
        elseif c == 'f' and str:sub(pos, pos+4) == 'false' then
            pos = pos + 5
            return false
        elseif c == 'n' and str:sub(pos, pos+3) == 'null' then
            pos = pos + 4
            return nil
        elseif c:match("[%d%-]") then
            return parse_num()
        end
        error("unexpected character in JSON: '" .. c .. "' at pos " .. pos)
    end

    return parse_val()
end

vtui.json = json

-- --- Host Binary Lookup ---
local function find_host_binary(explicit)
    if explicit and explicit ~= "" then return explicit end
    local env = os.getenv("VTUI_HOST_BIN")
    if env and env ~= "" then return env end

    local src = debug.getinfo(1, "S").source
    local current_dir = src:match("@?(.*/)") or "./"
    local base = current_dir .. "../../"
    local candidates = {
        base .. "cmd/vtui-host/vtui-host",
        base .. "bindings/cpp/build/vtui-host",
        base .. "bindings/c/build/vtui-host",
        base .. "bindings/build/vtui-host",
        base .. "build/vtui-host",
        base .. "vtui-host",
        "./vtui-host",
        "../vtui-host",
        "../../vtui-host",
        (os.getenv("HOME") or "") .. "/go/bin/vtui-host",
    }

    for _, cand in ipairs(candidates) do
        local f = io.open(cand, "r")
        if f then
            f:close()
            return cand
        end
    end

    -- Try auto-building if go compiler is present
    local target = base .. "vtui-host"
    os.execute("go build -o " .. target .. " " .. base .. "cmd/vtui-host 2>/dev/null")
    local f = io.open(target, "r")
    if f then
        f:close()
        return target
    end

    return "vtui-host"
end

local function try_load_native_clib(base_dir)
    local ok, mod = pcall(require, "vtui_lua")
    if ok and mod then return mod end

    local candidates = {
        base_dir .. "vtui_lua.so",
        base_dir .. "build/vtui_lua.so",
        base_dir .. "../build/vtui_lua.so",
        base_dir .. "../../build/vtui_lua.so",
    }
    for _, path in ipairs(candidates) do
        local f = io.open(path, "r")
        if f then
            f:close()
            local fn, err = package.loadlib(path, "luaopen_vtui_lua")
            if fn then
                return fn()
            end
        end
    end

    -- Auto-compile vtui_lua.so on the fly if gcc/clang is available
    local c_src = base_dir .. "src/vtui_lua.c"
    local f_src = io.open(c_src, "r")
    if f_src then
        f_src:close()
        local so_out = base_dir .. "vtui_lua.so"
        local inc_flags = "-I/usr/include/lua5.3 -I/usr/include/lua5.4 -I/usr/include/lua5.2 -I/usr/include/lua5.1 -I/usr/include/luajit-2.1 -I/usr/include/luajit-2.0 -I/usr/include/lua -I/usr/local/include/lua5.3 -I/usr/local/include/lua5.4 -I/usr/local/include/lua -I/opt/homebrew/include/lua5.3 -I/opt/homebrew/include/lua5.4 -I/opt/homebrew/include"
        local compile_cmd = string.format("gcc -O2 -fPIC -shared %s %s -o %s 2>/dev/null", inc_flags, c_src, so_out)
        os.execute(compile_cmd)
        local f_so = io.open(so_out, "r")
        if f_so then
            f_so:close()
            local fn = package.loadlib(so_out, "luaopen_vtui_lua")
            if fn then
                return fn()
            end
        end
    end

    return nil
end

-- --- Session Implementation ---
local Session = {}
Session.__index = Session

function Session.new(options)
    options = options or {}
    local self = setmetatable({}, Session)
    self.host_bin = find_host_binary(options.host_bin)
    self.backend = options.backend or os.getenv("VTUI_BACKEND") or "ansi"
    self.seq = 0
    self.proc_pid = nil
    return self
end

function Session:start()
    local current_file = debug.getinfo(1, "S").source:match("@?(.*/)") or "./"
    local native_clib = try_load_native_clib(current_file)
    if native_clib then
        self.clib_sess = native_clib.open(self.host_bin, self.backend)
        self.use_clib = true
        self:send({op = "hello", version = 1})
        local resp = self:recv()
        if not resp or resp.op == "error" then
            self:close()
            error((resp and resp.message) or "Handshake failed")
        end
        return
    end

    local has_ffi, ffi = pcall(require, "ffi")
    if has_ffi then
        pcall(function()
            ffi.cdef[[
                int socketpair(int domain, int type, int protocol, int sv[2]);
                int fork(void);
                int close(int fd);
                int dup2(int oldfd, int newfd);
                int execlp(const char *file, const char *arg0, ...);
                ssize_t read(int fd, void *buf, size_t count);
                ssize_t write(int fd, const void *buf, size_t count);
                int waitpid(int pid, int *status, int options);
                int kill(int pid, int sig);
                void _exit(int status);
            ]]
        end)
        local sv = ffi.new("int[2]")
        if ffi.C.socketpair(1, 1, 0, sv) == 0 then
            local pid = ffi.C.fork()
            if pid == 0 then
                ffi.C.close(sv[0])
                if sv[1] ~= 3 then
                    ffi.C.dup2(sv[1], 3)
                    ffi.C.close(sv[1])
                end
                local backend_arg = "--backend=" .. self.backend
                ffi.C.execlp(self.host_bin, self.host_bin, "--protocol-fd=3", backend_arg, nil)
                ffi.C._exit(127)
            elseif pid > 0 then
                ffi.C.close(sv[1])
                self.fd = sv[0]
                self.proc_pid = pid
                self.use_ffi = true
                self:send({op = "hello", version = 1})
                local resp = self:recv()
                if not resp or resp.op == "error" then
                    self:close()
                    error((resp and resp.message) or "Handshake failed")
                end
                return
            end
        end
    end

    error("vtui: could not initialize IPC channel (vtui_lua C module, LuaJIT FFI, or gcc required)")
end

function Session:send(msg)
    self.seq = self.seq + 1
    if not msg.seq then msg.seq = self.seq end
    local line = json.encode(msg) .. "\n"

    if self.use_clib and self.clib_sess then
        self.clib_sess:send(line)
    elseif self.use_ffi and self.fd then
        local ffi = require("ffi")
        ffi.C.write(self.fd, line, #line)
    end
    return self.seq
end

function Session:recv(timeout)
    local raw_line = nil
    if self.use_clib and self.clib_sess then
        raw_line = self.clib_sess:recv()
    elseif self.use_ffi and self.fd then
        local ffi = require("ffi")
        local buf = ffi.new("char[4096]")
        local line_parts = {}
        while true do
            local n = ffi.C.read(self.fd, buf, 1)
            if n <= 0 then break end
            local ch = string.char(buf[0])
            if ch == '\n' then break end
            table.insert(line_parts, ch)
        end
        if #line_parts > 0 then
            raw_line = table.concat(line_parts)
        end
    end

    if not raw_line or raw_line == "" then
        return nil
    end

    local json_start = raw_line:find("{")
    if json_start then
        raw_line = raw_line:sub(json_start)
    end

    local ok, res = pcall(json.decode, raw_line)
    if not ok then
        return nil
    end
    return res
end

function Session:mount(frame_id, tree)
    self:send({op = "mount", frameId = frame_id, tree = tree})
end

function Session:patch(frame_id, ops)
    self:send({op = "patch", frameId = frame_id, ops = ops})
end

function Session:message(title, text, buttons)
    self:send({op = "message", title = title, text = text, buttons = buttons or {"&Ok"}})
end

function Session:close()
    pcall(function() self:send({op = "quit"}) end)
    if self.use_clib and self.clib_sess then
        self.clib_sess:close()
        self.clib_sess = nil
    end
    if self.use_ffi and self.fd then
        local ffi = require("ffi")
        ffi.C.close(self.fd)
        if self.proc_pid then
            local status = ffi.new("int[1]")
            ffi.C.waitpid(self.proc_pid, status, 0)
        end
        self.fd = nil
    end
end

vtui.Session = Session

-- --- Immediate-Mode Ui Facade ---
local Ui = {}
Ui.__index = Ui

function Ui.new(session)
    local self = setmetatable({}, Ui)
    self.session = session
    self.container_stack = {}
    self.values = {}
    self.clicked_ids = {}
    self.mounted = false
    self.root_id = "mainDlg"
    self.current_root = nil
    return self
end

function Ui:dialog(title, w, callback)
    local node = {
        type = "Dialog",
        id = self.root_id,
        props = {title = title, autoSize = true, center = true},
        layout = {type = "VBox", spacing = 1, margins = {1, 2, 1, 2}},
        children = {},
    }
    table.insert(self.container_stack, node)
    if type(callback) == "function" then
        callback()
    end
    self.current_root = table.remove(self.container_stack)
    return self.current_root
end

function Ui:edit(label, default_val, id)
    local edit_id = id or ("edit_" .. label:gsub("&", ""):gsub("%s+", ""))
    if self.values[edit_id] == nil then
        self.values[edit_id] = default_val or ""
    end

    local group_node = {
        type = "Group",
        layout = {type = "Form", spacing = 1},
        children = {
            {type = "Label", props = {text = label, buddy = edit_id}},
            {type = "Edit", id = edit_id, props = {text = self.values[edit_id]}},
        },
    }

    if #self.container_stack > 0 then
        table.insert(self.container_stack[#self.container_stack].children, group_node)
    end
    return self.values[edit_id]
end

function Ui:button(text, id)
    local btn_id = id or ("btn_" .. text:gsub("&", ""):gsub("%s+", ""))
    local cmd_id = 1000 + math.abs(self:_hash(btn_id)) % 8000
    local node = {
        type = "Button",
        id = btn_id,
        props = {text = text, command = cmd_id},
    }
    if #self.container_stack > 0 then
        table.insert(self.container_stack[#self.container_stack].children, node)
    end
    if self.clicked_ids[btn_id] then
        self.clicked_ids[btn_id] = nil
        return true
    end
    return false
end

function Ui:checkbox(text, default_val, id)
    local chk_id = id or ("chk_" .. text:gsub("&", ""):gsub("%s+", ""))
    if self.values[chk_id] == nil then
        self.values[chk_id] = default_val or false
    end
    local node = {
        type = "Checkbox",
        id = chk_id,
        props = {text = text, state = self.values[chk_id] and 1 or 0},
    }
    if #self.container_stack > 0 then
        table.insert(self.container_stack[#self.container_stack].children, node)
    end
    return self.values[chk_id]
end

function Ui:message(title, text, buttons)
    self.session:message(title, text, buttons)
end

function Ui:_hash(str)
    local h = 0
    for i = 1, #str do
        h = (h * 31 + str:byte(i)) % 2147483647
    end
    return h
end

function Ui:_sync()
    if not self.current_root then return end
    if not self.mounted then
        self.session:mount(self.root_id, self.current_root)
        self.mounted = true
    end
    self.clicked_ids = {}
end

function Ui:_process_event(ev)
    if not ev then return end
    local op = ev.op
    if op == "command" and ev.srcId then
        self.clicked_ids[ev.srcId] = true
    elseif op == "changed" and ev.id then
        self.values[ev.id] = ev.value
    end
end

vtui.Ui = Ui

function vtui.log(...)
    local parts = {...}
    for i, p in ipairs(parts) do parts[i] = tostring(p) end
    io.stderr:write("[VTUI_LOG] " .. table.concat(parts, " ") .. "\n")
end

function vtui.run(ui_func, options)
    local session = Session.new(options)
    session:start()
    local u = Ui.new(session)

    local function step()
        ui_func(u)
        u:_sync()
    end

    step()

    while true do
        local ev = session:recv()
        if not ev then break end
        if ev.op == "closed" and ev.frameId == u.root_id then
            break
        end
        u:_process_event(ev)
        step()
    end

    session:close()
end

return vtui
