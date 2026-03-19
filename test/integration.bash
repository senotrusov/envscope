#!/usr/bin/env bash

# Set up strict mode unless explicitly disabled
if [[ "${ENVSCP_TEST_STRICT:-1}" == "1" ]]; then
  set -euo pipefail
fi

# Set up a mock HOME directory so the ~ expansion in test.conf is predictable
export HOME="$(pwd)/test"

FAILURES=0

echo "BASH: Running Error Handling Tests (Strict: ${ENVSCP_TEST_STRICT:-1})"

# Helper function to assert errors report correctly
assert_error() {
  local name="$1"
  local conf_file="$2"
  local expected_err="$3"
  
  local output code
  if output=$(bin/envscope -c "$conf_file" hook bash 2>&1 >/dev/null); then
    code=0
  else
    code=$?
  fi

  if [[ $code -eq 0 ]]; then
    echo "FAIL: $name - expected non-zero exit code"
    FAILURES=$((FAILURES + 1))
  elif [[ "$output" != *"$expected_err"* ]]; then
    echo "FAIL: $name - expected error containing '$expected_err', got '$output'"
    FAILURES=$((FAILURES + 1))
  else
    echo "PASS: $name"
  fi
}

# Helper function to assert output code is entirely empty on error protecting the shell evaluation
assert_error_output_empty() {
  local name="$1"
  local conf_file="$2"
  
  local stdout_output
  stdout_output=$(bin/envscope -c "$conf_file" hook bash 2>/dev/null) || true
  
  if [[ -n "$stdout_output" ]]; then
    echo "FAIL: $name - expected empty stdout on error, got: $stdout_output"
    FAILURES=$((FAILURES + 1))
  else
    echo "PASS: $name - stdout is empty on error"
  fi
}

assert_error "Missing config errors" "test/does-not-exist.conf" "no such file or directory"
assert_error_output_empty "Missing config stdout blank" "test/does-not-exist.conf"

assert_error "Variable without zone" "test/no-zone.conf" "variable definition without a preceding zone path"

assert_error "Invalid variable definition (missing '=')" "test/bad-var.conf" "invalid variable definition (missing '=')"
assert_error_output_empty "Bad config stdout blank" "test/bad-var.conf"

assert_error "Invalid variable definition (empty name)" "test/bad-var2.conf" "invalid variable name"

assert_error "Invalid variable name" "test/bad-var-name.conf" "invalid variable name"

assert_error "Unsupported double quotes" "test/bad-var-quotes.conf" "complex shell syntax in double quotes is not supported yet"

# Helper function to assert variables equal an expected value
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

# Helper function to assert a variable is empty or unset
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

echo "BASH: Running Integration Tests"

# 1. Initialize environment
mkdir -p "$HOME/other" \
         "$HOME/test/foo/bar" \
         "$HOME/test/tilde" \
         "$HOME/test/multi-1" \
         "$HOME/test/multi-2" \
         "$HOME/test/wildcard/foo/bar/deep" \
         "$HOME/test/wildcard/another/bar"

export PATH="/usr/bin:/bin"
ORIGINAL_PATH="$PATH"

# Source the generated hook using the newly built binary
# This simulates bashrc loading.
# The script claims to be `set -u` compatible, which is active here.
source <(bin/envscope -c test/test.conf hook bash)

# 2. Outside of any managed zone
cd "$HOME/other"
__envscope_hook
assert_empty "LOCALVAR outside" "${LOCALVAR:-}"
assert_eq "ROOT_VAR active everywhere" "${ROOT_VAR:-}" "root-zone"

# 3. Enter zone_0
cd "$HOME/test"
__envscope_hook
assert_eq "TESTROOT in zone_0" "${TESTROOT:-}" "testroot-value"
assert_eq "LOCALVAR in zone_0" "${LOCALVAR:-}" "test"
assert_eq "QUOTED_VAR in zone_0" "${QUOTED_VAR:-}" "val'withquote"
assert_eq "SPACED_VAR in zone_0" "${SPACED_VAR:-}" "val  spaced"

assert_eq "Tilde at start" "${TILDE_VAR:-}" "$HOME/foo"
assert_eq "Exact tilde" "${TILDE_VAR_EXACT:-}" "$HOME"
assert_eq "Tilde in middle (no expansion)" "${TILDE_VAR_MID:-}" "a~/foo"
assert_eq "Tilde after colon in non-PATH (no expansion)" "${TILDE_PATH_NOT_PATH:-}" ":/bin:~/foo"

if [[ -z "${DATE_VAR:-}" ]]; then
  echo "FAIL: DATE_VAR is empty"
  FAILURES=$((FAILURES + 1))
else
  echo "PASS: DATE_VAR is set to dynamic value"
fi

FIRST_DATE_VAR="${DATE_VAR:-}"

# Store the cached value for later verification
FIRST_DATE_VAR_CACHED="${DATE_VAR_CACHED:-}"
if [[ -z "$FIRST_DATE_VAR_CACHED" ]]; then
  echo "FAIL: DATE_VAR_CACHED is empty"
  FAILURES=$((FAILURES + 1))
else
  echo "PASS: DATE_VAR_CACHED initially set"
fi

# 4. Enter zone_1 (nested)
cd "$HOME/test/foo"
__envscope_hook
assert_eq "LOCALVAR in zone_1" "${LOCALVAR:-}" "test-foo"
assert_eq "TESTROOT in zone_1" "${TESTROOT:-}" "now-with-prefix-testroot-value"
assert_eq "PATH in zone_1" "${PATH:-}" "$HOME/prefix-that-does-not-exist:$ORIGINAL_PATH"

# 5. Enter zone_2 (deepest)
cd "$HOME/test/foo/bar"
__envscope_hook
assert_eq "LOCALVAR in zone_2" "${LOCALVAR:-}" "test-foo-bar"

# 4a. Enter zone_1 again (nested)
cd "$HOME/test/foo"
__envscope_hook
assert_eq "LOCALVAR in zone_1" "${LOCALVAR:-}" "test-foo"
assert_eq "TESTROOT in zone_1" "${TESTROOT:-}" "now-with-prefix-testroot-value"
assert_eq "PATH in zone_1" "${PATH:-}" "${HOME}/prefix-that-does-not-exist:$ORIGINAL_PATH"

# 6. Leave all zones (restore to outer)
cd "$HOME/other"
__envscope_hook
assert_empty "LOCALVAR restored" "${LOCALVAR:-}"
assert_empty "TESTROOT restored" "${TESTROOT:-}"
assert_empty "QUOTED_VAR restored" "${QUOTED_VAR:-}"
assert_empty "SPACED_VAR restored" "${SPACED_VAR:-}"
assert_empty "TILDE_VAR restored" "${TILDE_VAR:-}"
assert_empty "TILDE_VAR_EXACT restored" "${TILDE_VAR_EXACT:-}"
assert_empty "TILDE_VAR_MID restored" "${TILDE_VAR_MID:-}"
assert_empty "TILDE_PATH_NOT_PATH restored" "${TILDE_PATH_NOT_PATH:-}"
assert_eq "ROOT_VAR still active" "${ROOT_VAR:-}" "root-zone"
assert_eq "PATH restored" "${PATH:-}" "$ORIGINAL_PATH"

# 7. Re-enter zone_0 to verify caching behavior
cd "$HOME/test"
__envscope_hook

if [[ "${DATE_VAR:-}" == "$FIRST_DATE_VAR" ]]; then
  echo "FAIL: DATE_VAR did not change (expected dynamic re-evaluation, got '$FIRST_DATE_VAR')"
  FAILURES=$((FAILURES + 1))
else
  echo "PASS: DATE_VAR was re-evaluated dynamically"
fi

assert_eq "DATE_VAR_CACHED remains cached" "${DATE_VAR_CACHED:-}" "$FIRST_DATE_VAR_CACHED"

# 8. Manual override protection
# Modify a managed variable while inside the zone
export LOCALVAR="manual-override"
# Leave the zone
cd "$HOME/other"
__envscope_hook
# The manual override should be preserved (not reverted to empty)
assert_eq "LOCALVAR manual override protected" "${LOCALVAR:-}" "manual-override"

# 9. Tilde expansion in PATH
cd "$HOME/test/tilde"
__envscope_hook
assert_eq "Tilde after colon in PATH" "${PATH:-}" "$HOME/bin:/usr/bin:$HOME/local/bin:$HOME"

# 10. Absolute path zone
cd /
__envscope_hook
assert_eq "ROOT_VAR set in /" "${ROOT_VAR:-}" "root-zone"

# Go to a subdirectory of /
cd /etc
__envscope_hook
assert_eq "ROOT_VAR remains set in /etc" "${ROOT_VAR:-}" "root-zone"

# Re-enter a relative zone to make sure everything still works
cd "$HOME/test"
__envscope_hook
assert_eq "LOCALVAR in zone_0 after /" "${LOCALVAR:-}" "test"
assert_eq "ROOT_VAR is set in zone_0" "${ROOT_VAR:-}" "root-zone"

# 11. Multiple folders
cd "$HOME/test/multi-1"
__envscope_hook
assert_eq "MULTI_VAR in multi-1" "${MULTI_VAR:-}" "applied-to-both"

cd "$HOME/test/multi-2"
__envscope_hook
assert_eq "MULTI_VAR in multi-2" "${MULTI_VAR:-}" "applied-to-both"

# 12. Wildcard path
cd "$HOME/test/wildcard/foo/bar"
__envscope_hook
assert_eq "WILDCARD_VAR in wildcard/foo/bar" "${WILDCARD_VAR:-}" "matched"

cd "$HOME/test/wildcard/another/bar"
__envscope_hook
assert_eq "WILDCARD_VAR in wildcard/another/bar" "${WILDCARD_VAR:-}" "matched"

cd "$HOME/test/wildcard/foo/bar/deep"
__envscope_hook
assert_eq "WILDCARD_VAR inherited in deep" "${WILDCARD_VAR:-}" "matched"

# 13. Verify variables from multiple folders and wildcards correctly restore upon exit
cd "$HOME/other"
__envscope_hook
assert_empty "MULTI_VAR restored" "${MULTI_VAR:-}"
assert_empty "WILDCARD_VAR restored" "${WILDCARD_VAR:-}"

if [[ $FAILURES -gt 0 ]]; then
  echo "[X] BASH: $FAILURES test(s) failed."
  exit 1
else
  echo "[+] BASH: All integration tests passed."
  exit 0
fi
