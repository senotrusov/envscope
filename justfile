# Copyright 2026 Stanislav Senotrusov
#
# This work is dual-licensed under the Apache License, Version 2.0
# and the MIT License. Refer to the LICENSE file in the top-level directory
# for the full license terms.
#
# SPDX-License-Identifier: Apache-2.0 OR MIT

# Set the project name
project := "envscope"

# Release module
mod release

# List all available recipes
# This helper recipe itself is hidden from the list
_default:
  @just --list
  @just --list release

# Build and install the binary to /usr/local/bin
install: build
  sudo install --compare --mode 0755 --owner root --group root --target-directory /usr/local/bin bin/{{project}}

# Build the binary for the current OS/Arch
build:
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version={{`just release version`}}" -o bin/{{project}} ./cmd/{{project}}

# Remove all build artifacts
clean:
  rm -rf bin/{{project}}

# Format project files
format:
  mdformat --number *.md
  rg "[^\x00-\x7F]" && true

# Output key project file paths for LLM prompt context
context:
  #!/usr/bin/env bash
  set -euo pipefail
  echo '$ just test'
  printf "%s\n" \
    cmd/{{project}}/*.go \
    test/* \
    go.mod \
    justfile \
    README.md

short-context:
  #!/usr/bin/env bash
  set -euo pipefail
  printf "%s\n" \
    cmd/{{project}}/*.go \
    test/* \
    go.mod \
    justfile \
    README.md

# Run Go unit tests
unit-test:
  go test -v ./...

# Run the full test suite (Unit tests followed by Integration tests)
test: unit-test build
  #!/usr/bin/env bash
  set -u # Error if variable is undefined
  
  footer=$(mktemp)
  final_exit_status=0

  # Set color formatting if stdout is a terminal
  if [ -t 1 ]; then
    error_prefix="$(printf "setaf 9\nbold" | tput -S 2>/dev/null)"
    error_postfix="$(tput sgr 0 2>/dev/null)"
  fi

  # Helper function to run commands and capture exit status
  task() {
    "$@"
    if [[ $? -ne 0 ]]; then
      final_exit_status=1
      echo "${error_prefix}FAILED: $*${error_postfix}"
      echo "${error_prefix}FAILED: $*${error_postfix}" >> "$footer"
    fi
  }

  generate() {
    bin/envscope -c test/test.conf "$@" | sed "s|${HOME}/|/home/user/|g"
  }

  test_integration() {
    task bash test/integration.bash
    task bash test/integration-home.bash
    task zsh test/integration.zsh
    task zsh test/integration-home.zsh
  }

  # Fish tests are run once as Fish lacks a direct -u equivalent
  task fish test/integration.fish
  task fish test/integration-home.fish

  # Run Integration Tests: Standard Mode
  export ENVSCP_TEST_STRICT=0
  test_integration

  # Run Integration Tests: Strict Mode
  export ENVSCP_TEST_STRICT=1
  test_integration
  
  # Generate documentation artifacts
  task generate hook bash > test/hook.bash
  task generate -reportvars hook bash > test/hook-reportnames.bash
  task generate hook fish > test/hook.fish
  task generate -reportvars hook fish > test/hook-reportnames.fish
  task generate hook zsh > test/hook.zsh
  task generate -reportvars hook zsh > test/hook-reportnames.zsh

  if [[ -s "$footer" ]]; then
    cat "$footer"
  fi

  rm -f "$footer"
  exit "$final_exit_status"

# Start a nested Bash shell with the test configuration hook pre-loaded
test-shell: build
  #!/usr/bin/env bash
  set -euo pipefail
  export HOME="$PWD/test"
  bash --rcfile <(
    echo 'export PS1="${PS1:-}(envscope-test) "'
    bin/envscope -c test/test.conf -reportvars hook bash
    echo 'cd $HOME'
  ) -i || true

# Run shell performance benchmark
shell-benchmark:
  bash benchmark/var-indirection.bash
  fish benchmark/var-indirection.fish
  zsh  benchmark/var-indirection.zsh
  bash benchmark/var-access.bash
  zsh  benchmark/var-access.zsh
