function PLUGIN:PackageInstall(ctx)
  local names = {}
  for _, pkg in ipairs(ctx.packages) do
    table.insert(names, pkg.name)
  end
  local flags = ""
  if ctx.dry_run then flags = flags .. " --dry-run" end
  if ctx.update then flags = flags .. " --update" end
  local cmd = "dotdrift paru install" .. flags .. " " .. table.concat(names, " ") .. " 2>&1"
  local pipe = io.popen(cmd, "r")
  if pipe then
    for line in pipe:lines() do
      io.stderr:write(line .. "\n")
      io.stderr:flush()
    end
    pipe:close()
  end
  return {}
end
