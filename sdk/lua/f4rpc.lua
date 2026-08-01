-- F4-RPC SDK for Lua
-- Compatible with Lua 5.1+, LuaJIT
-- Dependency: lua-MessagePack

local mp = require('MessagePack')

local f4rpc = {
    handlers = {},
    pending = {},
    nextID = 1,
    SHIFT = 16,
    CTRL = 8,
    ALT = 2,
}

-- Disable output buffering for real-time RPC communication
io.stdout:setvbuf("no")

-- [Windows Specific] Binary mode negotiation.
-- Windows CRLF translation and 0x1A (EOF) character will corrupt MessagePack data.
if package.config:sub(1,1) == '\\' then
    local ok, ffi = pcall(require, "ffi")
    if ok then
        -- Use FFI to force binary mode on standard streams if running LuaJIT
        ffi.cdef[[
            int _setmode(int fd, int mode);
        ]]
        local O_BINARY = 0x8000
        ffi.C._setmode(0, O_BINARY) -- stdin
        ffi.C._setmode(1, O_BINARY) -- stdout
    end
end

-- mp.unpacker needs a function that returns bytes.
-- We use a closure around io.stdin:read(1).
local unpack_cursor = mp.unpacker(function()
    return io.stdin:read(1)
end)

-- register maps a method name (e.g. "VFS.ReadDir") to a Lua function.
function f4rpc.register(method, handler)
    f4rpc.handlers[method] = handler
end

-- Internal: Read one complete MessagePack object.
function f4rpc.read_msg()
    -- unpack_cursor returns (status, result, byte_offset) or just (result)
    -- depending on the internal implementation of lua-MessagePack.
    local res1, res2 = unpack_cursor()
    if type(res1) == "table" then return res1 end
    if type(res2) == "table" then return res2 end
    return nil
end

-- Internal: Route and handle messages.
function f4rpc.handle_msg(msg)
    if msg.t == 0 then -- Request from Core
        local handler = f4rpc.handlers[msg.m]
        local resp = { t = 1, i = msg.i }

        if not handler then
            resp.e = "method not found: " .. tostring(msg.m)
        else
            local ok, res = pcall(handler, msg.d)
            if not ok then
                resp.e = tostring(res)
            else
                resp.d = res
            end
        end

        io.stdout:write(mp.pack(resp))
        io.stdout:flush()
    elseif msg.t == 1 then -- Response to a Call we made
        f4rpc.pending[msg.i] = msg
    end
end

-- call invokes a method on the f4 Core (Host API).
-- This is synchronous: it waits for the response while processing intermediate requests.
function f4rpc.call(method, params)
    local id = f4rpc.nextID
    f4rpc.nextID = f4rpc.nextID + 1

    local req = {
        t = 0,
        i = id,
        m = method,
        d = params
    }

    io.stdout:write(mp.pack(req))
    io.stdout:flush()

    while true do
        if f4rpc.pending[id] then
            local resp = f4rpc.pending[id]
            f4rpc.pending[id] = nil
            if resp.e and resp.e ~= "" then
                error("F4-RPC Host Error: " .. resp.e)
            end
            return resp.d
        end

        local msg = f4rpc.read_msg()
        if not msg then
            error("F4-RPC: Unexpected EOF while waiting for response")
        end
        f4rpc.handle_msg(msg)
    end
end

-- serve enters the main loop, processing requests from f4.
function f4rpc.serve()
    while true do
        local msg = f4rpc.read_msg()
        if not msg then break end
        f4rpc.handle_msg(msg)
    end
end

return f4rpc