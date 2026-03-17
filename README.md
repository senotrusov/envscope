<!--
Copyright 2026 Stanislav Senotrusov

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the LICENSE file in the top-level directory
for the full license terms.

SPDX-License-Identifier: Apache-2.0 OR MIT
-->

# envscope

`envscope` is a environment variable manager that lets you define directory-specific variables in a single, centralized configuration file.

Unlike many environment managers that search for `.env` files in every directory you enter, `envscope` takes a different approach. It parses your global configuration once when the shell starts and generates optimized shell code that integrates with your prompt. This keeps project directories free of hidden configuration files while reducing runtime complexity.

## Features

* **Centralized configuration:** Manage all directory rules in `~/.config/envscope/main.conf`. A different path can be specified via an option.
* **Fast:** After initialization, everything runs as pure Bash logic. No external binaries are invoked when changing directories, and no processes are spawned unless explicitly requested in the configuration through dynamic variables.
* **Hierarchical:** Variables automatically inherit from parent directories.
* **Robust:** Compatible with `set -euo pipefail`.
* **Prepend support:** Easily prepend values to `PATH` or other variables using the `+` prefix.
* **Dynamic value caching:** Dynamic expressions evaluated dynamically each time the scope changes by default. You can opt in to caching for expensive commands or commands that require user interaction (for example hardware-backed credential managers) by adding a `# cache` comment.
* **Override awareness:** If you manually `export` a variable while inside a managed directory, `envscope` detects the change and does not overwrite it when you leave that scope.

## Installation

1. **Build and install from source:**

   ```bash
   just install
   ```

2. **Initialize the configuration:**
   Create the configuration directory and file:

   ```bash
   mkdir -p ~/.config/envscope
   touch ~/.config/envscope/main.conf
   ```

3. **Enable it in your shell:**
   Add the following line to the end of your `~/.bashrc`:

   ```bash
   # Load envscope hook if available
   if command -v envscope &>/dev/null; then
     builtin source <(envscope hook bash)
   fi
   ```

## Configuration Format

The configuration file is located at `~/.config/envscope/main.conf`.

- **Paths:** Start at the beginning of the line. Paths starting with `/` are absolute. All other paths are relative to your home directory. You can use a single dot `.` to represent your home directory itself.
- **Variables:** Must be indented with at least one space or tab. Variables are validated to ensure they conform to POSIX standard naming conventions (`^[a-zA-Z_][a-zA-Z0-9_]*$`).
- **Prepending:** Use `+VAR=value` to prepend to an existing variable. If the variable is `PATH`, it automatically handles the `:` separator.
- **Values and Quoting:**
  - **Plain text:** Values are treated as literal strings and do not require surrounding quotes. They are safely enclosed in single quotes when executed, preserving spaces and special characters. Double quotes around the entire value (e.g., `VAR="val"`) are not currently supported and will return an error.
  - **Tilde Expansion:** A tilde (`~`) at the beginning of an unquoted value expands to your home directory (e.g., `VAR=~/foo` becomes `/home/user/foo`). Tildes in the middle of a string are treated literally (`VAR=a~/foo`).
  - **PATH Special Case:** For the `PATH` variable specifically, tildes that immediately follow a colon `:` are also expanded, supporting standard list formats like `PATH=~/bin:/usr/bin:~/.local/bin`.
  - **Dynamic Values:** You can evaluate Bash commands by wrapping the *entire* value in `$()`, such as `$(command)`. These are safely evaluated at runtime.
  - **Caching:** By default, dynamic values are re-evaluated on each scope change. To cache the result for the current shell session, add a `# cache` comment at the end of the line.

### Example `main.conf`

```text
projects/work
  PGDATABASE=work_db
  API_KEY=$(passage show my/work-api-key) # cache

projects/work/microservice-a
  PGDATABASE=service_a_db
  +PATH=~/projects/work/microservice-a/bin

sandbox
  TEMP_ENV=true
```

In the example above, when you `cd` into `~/projects/work/microservice-a`:
- `PGDATABASE` will be `service_a_db` (overriding the parent).
- `API_KEY` will be inherited from `projects/work`. Because of the `# cache` comment, the `passage` command will only be run once per shell session.
- `PATH` will be prepended with the new `bin` directory.

## How it works

1. **Startup:** When you open a new shell, `envscope hook` runs. It reads your `main.conf`, determines the parent-child relationships between your paths, and generates a series of shell functions.
2. **Tracking:** Every time your prompt is displayed, the `__envscope_hook` function checks `$PWD` against the generated path rules to find the most specific match.
3. **State Management:**
   - When entering a managed directory for the first time, it saves the "outer" (original) state of any variable it is about to change.
   - When changing directories, it completely restores the outer environment and then re-applies the full stack of variables for the new location, from the top-level parent down to the most specific child.
   - When leaving all managed directories, it restores all variables to their original values (or unsets them if they didn't exist).
4. **Safety:** If the current value of a variable does not match the value `envscope` last set, the tool assumes you have manually changed it and refuses to touch it upon leaving a zone, preserving your manual overrides.

## License

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the [LICENSE](LICENSE) file in the top-level
directory for the full license terms.

## Get involved

See the [CONTRIBUTING](CONTRIBUTING.md) file for guidelines
on how to contribute, and the [CONTRIBUTORS](CONTRIBUTORS.md)
file for a list of contributors.
