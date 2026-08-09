function PLUGIN:PackageInstalled(ctx)
  local names = {}
  for _, pkg in ipairs(ctx.packages) do
    table.insert(names, pkg.name)
  end
  local handle = io.popen("dotdrift paru installed " .. table.concat(names, " "))
  local output = handle:read("*a")
  handle:close()
  local result = { packages = {} }
  for line in output:gmatch("[^\n]+") do
    local name, state, version = line:match("([^\t]+)\t([^\t]+)\t?([^\t]*)")
    if name and state then
      local entry = { name = name, state = state }
      if version and version ~= "" then entry.version = version end
      table.insert(result.packages, entry)
    end
  end
  return result
end
