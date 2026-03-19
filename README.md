<!--
Copyright 2026 Stanislav Senotrusov

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the LICENSE file in the top-level directory
for the full license terms.

SPDX-License-Identifier: Apache-2.0 OR MIT
-->

# envscope

`envscope` is a fast, centralized environment variable manager for your shell. It allows you to automatically load and unload environment variables based on your current directory.

## The `envscope` approach

Unlike traditional environment managers that require you to scatter hidden configuration files across every project directory and spawn external processes every time you navigate your filesystem, `envscope` takes a different approach:

1. **Centralized configuration:** You define all your directory-specific variables in a single global configuration file (`~/.config/envscope/main.conf`). Your project directories remain clean.
2. **Zero-overhead execution:** When your shell starts, `envscope` parses your configuration exactly once and compiles it into optimized, native shell code (Bash, Zsh, or Fish). 
3. **Pure shell logic:** Changing directories triggers pure shell functions to evaluate your current path and apply the correct variables. No external binaries are invoked during navigation, ensuring your prompt remains instant.

## Quick example

Here is what your `~/.config/envscope/main.conf` might look like:

```text
projects/work
  AWS_PROFILE=work-prod
  API_KEY=$(passage show my/work-api-key) # cache

projects/work/microservice-a
  PGDATABASE=service_a_db
  +PATH=~/projects/work/microservice-a/bin

sandbox
  TEMP_ENV=true
```

With this configuration:
* Navigating anywhere inside `~/projects/work` sets `AWS_PROFILE` and fetches your `API_KEY`. The `# cache` directive ensures the secure vault is only queried once per shell session.
* Moving deeper into `~/projects/work/microservice-a` inherits the parent variables, adds `PGDATABASE`, and safely prepends a local `bin` folder to your `PATH`.
* Leaving these directories automatically cleanly restores your previous `PATH` and unsets the project-specific variables.

## Features

* **Hierarchical inheritance:** Variables automatically cascade. Deeper directories inherit variables from their parent directories, and can override them or add new ones.
* **Prepending:** Easily prepend values to existing variables (like `PATH`) using the `+` prefix.
* **Dynamic value caching:** Evaluate shell commands dynamically for variable values. You can opt-in to session-caching for expensive commands (e.g., querying hardware-backed credential managers).
* **Manual override protection:** If you manually `export` a managed variable while inside a directory zone, `envscope` detects the change and will not wipe out your manual override when you leave the zone.
* **Robust & safe:** Fully compatible with strict shell environments (`set -euo pipefail`).

## Installation & setup

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
   Add the appropriate line to the end of your shell's configuration file:

   **Bash** (`~/.bashrc`):
   ```bash
   # Load envscope hook if available
   if command -v envscope &>/dev/null; then
     builtin source <(envscope hook bash)
   fi
   ```

   **Zsh** (`~/.zshrc`):
   ```zsh
   # Load envscope hook if available
   if command -v envscope &>/dev/null; then
     builtin source <(envscope hook zsh)
   fi
   ```

   **Fish** (`~/.config/fish/config.fish`):
   ```fish
   # Load envscope hook if available
   if command -v envscope &>/dev/null
     envscope hook fish | source
   end
   ```

> **Note on reloading:** Because `envscope` compiles your configuration into shell functions at startup, any changes you make to `main.conf` will not take effect immediately. You must restart your shell to apply updates.

### Reporting changes

To get feedback in your terminal whenever `envscope` modifies your environment, update the initialization line in your shell config to include the `-reportvars` flag. This will print messages to `stderr` whenever you cross directory boundaries:
- `envscope: added PGDATABASE`
- `envscope: removed TEMP_ENV`

## Configuration guide

The configuration file is located at `~/.config/envscope/main.conf`.

### Paths & zones
- **Home relative:** Paths that start at the beginning of the line are treated as relative to your home directory (`~`). You can use a single dot `.` to represent your home directory itself.
- **Absolute:** Paths starting with `/` are treated as absolute system paths.
- **Wildcards:** Paths can include the `*` wildcard character, which matches any sequence of characters (including slashes and empty strings). Directory names containing newline characters are not supported.
  ```text
  projects/*/src
    ENV_TYPE=development
  ```
- **Multiple Folders:** You can apply the same set of variables to multiple directory paths by listing the paths sequentially before the indented variables block:
  ```text
  projects/backend-a
  projects/backend-b
    DB_PORT=5432
  ```

### Variables
- **Indentation:** Variable definitions **must** be indented with at least one space or tab under their respective path.
- **Naming:** Variables are strictly validated against POSIX standard naming conventions (`^[a-zA-Z_][a-zA-Z0-9_]*$`).
- **Prepending (`+`):** Use `+VAR=value` to safely prepend to an existing variable. If the variable is `PATH`, `envscope` automatically handles the `:` separator syntax for you.

### Values and quoting
- **Plain text:** Values are treated as literal strings and do not require surrounding quotes. They are safely enclosed in single quotes internally when executed, preserving spaces and special characters. Double quotes around the entire value (e.g., `VAR="val"`) are not currently supported and will return an error.
- **Tilde expansion:** A tilde (`~`) at the beginning of an unquoted value expands to your home directory (e.g., `VAR=~/foo` becomes `/home/user/foo`). Tildes in the middle of a string are treated literally (`VAR=a~/foo`).
- **PATH special Case:** For the `PATH` variable specifically, tildes that immediately follow a colon `:` are also expanded, naturally supporting standard list formats like `PATH=~/bin:/usr/bin:~/.local/bin`.

### Dynamic execution & caching
You can evaluate shell commands to dynamically generate variable values by wrapping the *entire* value in `$()`.

```text
projects/work
  # Evaluated every time you enter the directory
  CURRENT_DATE=$(date)
  
  # Evaluated once per shell session and cached
  API_KEY=$(passage show my/work-api-key) # cache
```
By default, dynamic values are executed and re-evaluated **every time** you enter the directory zone. If the command is slow or requires user input (like a password prompt or a hardware token tap), append a `# cache` comment exactly at the end of the line. `envscope` will execute the command the first time it is needed and cache the result in memory for the lifetime of that shell session.

## How it works under the hood

1. **Path specificity:** When matching your current `$PWD` against the zones defined in your config, `envscope` always chooses the **most specific match** (the longest matching path). It then builds a hierarchy from the root parent down to the matched child path.
2. **State management:**
   - When entering a managed directory sequence for the first time, it snapshots the "outer" (original) state of any variable it is about to modify.
   - During directory changes, it completely unloads the current zone by reverting to the saved outer state, then recalculates and applies the full stack of variables for the new location (parent variables first, then child overrides).
   - Leaving all managed directories entirely restores all variables back to their original snapshot (unsetting them if they did not previously exist).
3. **Safety & overrides:** Before removing or restoring a variable, `envscope` checks its current value against the value it originally set. If the values differ, the tool assumes you have manually executed an `export VAR=...` override and refuses to touch it, gracefully preserving your local adjustments.
4. **The `//` caveat:** `envscope` enforces a strict matching logic by normalizing `$PWD` with a trailing slash. If you mistakenly navigate to an irregular root structure like `//home/user` (two leading slashes), `envscope` may not correctly trigger your configurations. When navigating using absolute paths, be mindful of how many leading slashes you include, since `cd /home/user` and `cd //home/user` can have different meanings.

## License

This work is dual-licensed under the Apache License, Version 2.0
and the MIT License. Refer to the [LICENSE](LICENSE) file in the top-level
directory for the full license terms.

## Get involved

See the [CONTRIBUTING](CONTRIBUTING.md) file for guidelines
on how to contribute, and the [CONTRIBUTORS](CONTRIBUTORS.md)
file for a list of contributors.
