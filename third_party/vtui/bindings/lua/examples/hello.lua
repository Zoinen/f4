local script_dir = debug.getinfo(1, "S").source:match("@?(.*/)") or ""
package.path = script_dir .. "../?.lua;" .. script_dir .. "?.lua;" .. package.path

local vtui = require("vtui")

vtui.run(function(u)
    u:dialog(" Hello vtui ", 40, function()
        local name = u:edit("&Name:", "Type here...")
        if u:button("&Ok") then
            u:message(" Result ", "You typed:\n" .. name)
        end
    end)
end)
