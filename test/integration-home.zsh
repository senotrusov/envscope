#!/usr/bin/env zsh

if [[ "${ENVSCP_TEST_STRICT:-1}" == "1" ]]; then
  setopt NO_UNSET ERR_EXIT PIPE_FAIL
fi

export HOME="$(pwd)/test"
FAILURES=0

mkdir -p "$HOME/sub"

assert_eq() {
  local name="$1"
  local actual="$2"
  local expected="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL: $name - expected '$expected', got '$actual'"
    FAILURES=$((FAILURES + 1))
  else
    echo "PASS: $name"
  fi
}

assert_empty() {
  local name="$1"
  local actual="${2:-}"
  if [[ -n "$actual" ]]; then
    echo "FAIL: $name - expected empty, got '$actual'"
    FAILURES=$((FAILURES + 1))
  else
    echo "PASS: $name"
  fi
}

echo "ZSH: Running Home Integration Tests (Strict: ${ENVSCP_TEST_STRICT:-1})"

source <(bin/envscope -c test/home.conf hook zsh)

# 1. Start outside managed zones (e.g., /tmp)
cd /tmp
__envscope_hook
assert_empty "HOME_VAR outside" "${HOME_VAR:-}"

# 2. Enter home (.)
cd "$HOME"
__envscope_hook
assert_eq "HOME_VAR in ~" "${HOME_VAR:-}" "home-base"

# 3. Enter sub (implicitly resolves to ~/sub relative to .)
cd "$HOME/sub"
__envscope_hook
assert_eq "HOME_VAR inherited in sub" "${HOME_VAR:-}" "home-base"
assert_eq "SUB_VAR in sub" "${SUB_VAR:-}" "sub-level"

# 4. Leave to /etc (leaving all zones)
cd /etc
__envscope_hook
assert_empty "HOME_VAR restored" "${HOME_VAR:-}"
assert_empty "SUB_VAR restored" "${SUB_VAR:-}"

if [[ $FAILURES -gt 0 ]]; then
  echo "[X] $FAILURES home test(s) failed."
  exit 1
else
  echo "[+] All home integration tests passed."
  exit 0
fi
