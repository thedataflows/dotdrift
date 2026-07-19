#!/bin/sh
# dotdrift end-to-end scenario. Runs INSIDE the e2e container as root.
# Exercises: real mise bootstrap (mise.run), real apt install, real dotfile
# linking, real hooks-as-mise-tasks, resume, and profile-pollution checks.
# Any failed assertion prints "FAIL: <reason>" and exits non-zero.

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

step() {
	echo
	echo "== $* =="
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
echo "$PLAN" | grep -q "curl" || fail "plan output does not mention package curl"
echo "$PLAN" | grep -q "\.demorc" || fail "plan output does not mention ~/.demorc"
echo "$PLAN" | grep -q "pre-hook" || fail "plan output does not list the pre hook"
echo "$PLAN" | grep -q "post-hook" || fail "plan output does not list the post hook"

# The image ships curl only so the mise.run bootstrap above can download
# mise. Purge it so the packages step must perform a REAL install against
# the (intentionally empty) apt index.
step "purge image curl (force a real install)"
apt-get purge -y curl > /dev/null || fail "could not purge curl"
if dpkg -l curl > /dev/null 2>&1; then
	fail "curl still installed after purge"
fi

# --- apply ---------------------------------------------------------------
step "apply"
dotdrift apply --profile /profile --yes || fail "apply exited non-zero"

# (a) real package-manager verification
dpkg -l curl > /dev/null 2>&1 || fail "dpkg: curl is not installed after apply"

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

# (d) state file reports the pipeline complete
STATE=$(ls /root/.local/state/dotdrift/profiles/*/state.json 2> /dev/null) || fail "no state.json under /root/.local/state/dotdrift/profiles"
[ -n "$STATE" ] || fail "no state.json under /root/.local/state/dotdrift/profiles"
grep -Eq '"status": *"complete"' "$STATE" || fail "state file $STATE does not contain \"status\":\"complete\""

# (e) resume: second apply is a no-op and stays complete
step "apply again (resume no-op)"
dotdrift apply --profile /profile --yes || fail "second apply exited non-zero"
grep -Eq '"status": *"complete"' "$STATE" || fail "state no longer complete after second apply"

# (f) onboard/apply produced no runtime files inside the profile
if find /profile/modules -name .mise -print -quit | grep -q .; then
	fail ".mise runtime dir pollutes /profile/modules"
fi

echo
echo "SCENARIO PASS"
