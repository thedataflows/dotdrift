# Status shows a configuration notice for other accounts, not drift sections

ADR-0005 gave `status` a per-account drift view: every existing account with
a user layer got its own probed section. Within a day of real use it proved
too noisy to be useful: every section depended on sudo working per probe,
tool probes ran other accounts' mise with all of its cwd/config sensitivity
(issue 0039's class of failure surfaced twice per run), and a converged or
unprobeable account produced screens of `missing`/`(?)` lines that told the
invoking user nothing they could act on — only that account can apply, and
the report never said so plainly.

We decided: **status says, not shows.** After the invoking account's report,
one notice lists each existing account that has configuration on this
machine (`users/<name>/` with at least one module or a config-only
`dotdrift.toml`) with the exact apply command — `sudo dotdrift apply` for
the uid-0 account, `sudo -iu <name> dotdrift apply` for any other (a login
shell, so the account's own PATH applies) — plus one trailing reminder that
each account needs dotdrift resolvable on its PATH (system-wide install,
e.g. via mise, system scope). No sudo, no per-account mise, no drift lines,
no visibility probes.

Apply semantics are unchanged and remain the load-bearing rule: converging
another account means running dotdrift as that account. This ADR supersedes
ADR-0005's reporting half; its apply half was never in question.
