local script_dir = debug.getinfo(1, "S").source:match("@?(.*/)") or ""
package.path = script_dir .. "../?.lua;" .. script_dir .. "?.lua;" .. package.path

local vtui = require("vtui")

local MockSession = {}
MockSession.__index = MockSession

function MockSession.new()
    return setmetatable({mounted = {}, messages = {}}, MockSession)
end

function MockSession:mount(frame_id, tree)
    table.insert(self.mounted, {frameId = frame_id, tree = tree})
end

function MockSession:message(title, text, buttons)
    table.insert(self.messages, {title = title, text = text, buttons = buttons})
end

function MockSession:send(msg) end
function MockSession:recv() return nil end
function MockSession:close() end

local function assert_eq(got, want, msg)
    if got ~= want then
        error(string.format("ASSERTION FAILED: %s (got: %s, want: %s)", msg, tostring(got), tostring(want)))
    end
end

-- 1. Test JSON codec
local encoded = vtui.json.encode({hello = "world", count = 42, active = true, list = {1, 2, 3}})
local decoded = vtui.json.decode(encoded)
assert_eq(decoded.hello, "world", "JSON decode string")
assert_eq(decoded.count, 42, "JSON decode number")
assert_eq(decoded.active, true, "JSON decode boolean")
assert_eq(#decoded.list, 3, "JSON decode array size")

-- 2. Test Immediate-Mode Tree Construction
local session = MockSession.new()
local u = vtui.Ui.new(session)

u:dialog(" Test Dialog ", 40, function()
    local name = u:edit("&Name:", "Alice")
    assert_eq(name, "Alice", "Default edit text")
    local clicked = u:button("&Submit")
    assert_eq(clicked, false, "Initial button click state")
end)

u:_sync()

assert_eq(#session.mounted, 1, "Mounted frame count")
assert_eq(session.mounted[1].frameId, "mainDlg", "Frame ID")
assert_eq(session.mounted[1].tree.type, "Dialog", "Root type")
assert_eq(#session.mounted[1].tree.children, 2, "Dialog children count")

-- 3. Test Event Processing & Reactive Update
u:_process_event({op = "changed", id = "edit_Name:", value = "Bob"})
u:_process_event({op = "command", srcId = "btn_Submit", cmd = 1000})

local submitted = false
u:dialog(" Test Dialog ", 40, function()
    local name = u:edit("&Name:", "Alice")
    assert_eq(name, "Bob", "Updated edit text after event")
    if u:button("&Submit") then
        submitted = true
    end
end)

assert_eq(submitted, true, "Button clicked after command event")

print("Lua bindings unit test passed.")
