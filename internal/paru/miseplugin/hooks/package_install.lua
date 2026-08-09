function PLUGIN:PackageInstall(ctx)
  local names = {}
  for _, pkg in ipairs(ctx.packages) do
    table.insert(names, pkg.name)
  end
  local flags = ""
  if ctx.dry_run then flags = flags .. " --dry-run" end
  if ctx.update then flags = flags .. " --update" end
  os.execute("dotdrift paru install" .. flags .. " " .. table.concat(names, " "))
  return {}
end
