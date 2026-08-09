#!/bin/sh
# dotdrift end-to-end scenario. Runs INSIDE the e2e container as root.
# Exercises: real mise bootstrap (mise.run), a real package install (apt on
# debian-family, pacman/paru on CachyOS), real dotfile linking, real
# hooks-as-mise-tasks, resume, and profile-pollution checks.
# Any failed assertion prints "FAIL: <reason>" and exits non-zero.

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

step() {
	echo
	echo "== $* =="
}

# Package-manager verbs: debian-family images use dpkg; the CachyOS image uses
# pacman (dotdrift installs via paru, which `pacman -Q` then observes). jq is a
# leaf package absent from every base image, so `dotdrift apply` performs a
# REAL install (decoupled from curl, which the image keeps only for the
# mise.run bootstrap). On CachyOS curl is a hard dependency of pacman itself
# and cannot be removed, so it cannot serve as the purge/reinstall target there.
have_pacman() { command -v pacman >/dev/null 2>&1; }
TEST_PKG=jq
# Exit 0 when TEST_PKG IS installed.
pkg_present() {
	if have_pacman; then pacman -Q "$TEST_PKG" >/dev/null 2>&1
	else dpkg -l "$TEST_PKG" >/dev/null 2>&1; fi
}

step "detect"
dotdrift detect || fail "dotdrift detect exited non-zero"

# --- onboard -------------------------------------------------------------
step "onboard"
echo "live-config 1" > /root/.liverc
dotdrift onboard --yes --profile /profile /root/.liverc || fail "onboard exited non-zero"
[ -f /profile/modules/liverc/module.toml ] || fail "onboard did not materialize modules/liverc/module.toml"
[ -f /profile/modules/liverc/home/.liverc ] || fail "onboard did not copy the live file into the module"
[ -L /root/.liverc ] || fail "onboard did not link /root/.liverc"

# --- plan ----------------------------------------------------------------
step "plan"
PLAN=$(dotdrift plan --profile /profile) || fail "plan exited non-zero"
echo "$PLAN"
echo "$PLAN" | grep -q "$TEST_PKG" || fail "plan output does not mention package $TEST_PKG"
echo "$PLAN" | grep -q "\.demorc" || fail "plan output does not mention ~/.demorc"
echo "$PLAN" | grep -q "pre-hook" || fail "plan output does not list the pre hook"
echo "$PLAN" | grep -q "post-hook" || fail "plan output does not list the post hook"

# The test package ships in none of the base images, so `dotdrift apply` must
# perform a REAL install (curl stays installed only for the mise.run bootstrap
# above). Asserting it absent beforehand proves the install is not a no-op.
step "verify $TEST_PKG absent before apply"
pkg_present && fail "$TEST_PKG unexpectedly present in base image"

# --- apply ---------------------------------------------------------------
step "apply"
dotdrift apply --profile /profile --yes || fail "apply exited non-zero"

# (a) real package-manager verification
pkg_present || fail "$TEST_PKG is not installed after apply"

# (b) dotfile symlink resolves into the profile's demo module
[ -L /root/.demorc ] || fail "/root/.demorc is not a symlink"
case "$(readlink -f /root/.demorc)" in
	/profile/modules/demo/*) ;;
	*) fail "/root/.demorc does not resolve into /profile/modules/demo" ;;
esac

# (c) hooks ran as mise tasks
[ -f /tmp/hooks.log ] || fail "/tmp/hooks.log missing: hooks did not run"
grep -q "pre-hook" /tmp/hooks.log || fail "pre-hook missing from /tmp/hooks.log"
grep -q "post-hook" /tmp/hooks.log || fail "post-hook missing from /tmp/hooks.log"

# system-scope dotfile copied to /etc (covers the EUID==0 path: containers run as root, no sudo needed)
grep -q "sysdemo = true" /etc/sysdemo.conf || fail "/etc/sysdemo.conf missing or wrong content (system scope)"

# (d) a successful apply leaves no state file: the cursor is deleted on
# completion, so the next apply starts from the beginning.
if ls /root/.local/state/dotdrift/profiles/*/state.json >/dev/null 2>&1; then
	fail "state.json must not exist under /root/.local/state/dotdrift/profiles after a successful apply"
fi

# (e) resume: a second apply runs the full pipeline again (no cursor to skip)
# and still leaves no state file on disk.
step "apply again (full pipeline, no state file)"
dotdrift apply --profile /profile --yes || fail "second apply exited non-zero"
if ls /root/.local/state/dotdrift/profiles/*/state.json >/dev/null 2>&1; then
	fail "state.json must not exist under /root/.local/state/dotdrift/profiles after the second apply"
fi

# (f) onboard/apply produced no runtime files inside the profile
if find /profile/modules -name .mise -print -quit | grep -q .; then
	fail ".mise runtime dir pollutes /profile/modules"
fi

echo
echo "SCENARIO PASS"
