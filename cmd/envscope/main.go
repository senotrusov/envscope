package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EnvVar represents a parsed environment variable configuration.
type EnvVar struct {
	Name      string
	Value     string
	Prepend   bool
	IsPath    bool
	IsDynamic bool
	Cache     bool
}

// Zone represents a single path and its variable definitions.
type Zone struct {
	Path     string
	ID       string
	ParentID string
	Vars     []EnvVar
}

// main coordinates the initialization, parsing, and bash output generation.
func main() {
	configFlag := flag.String("c", "", "path to the configuration file")
	reportFlag := flag.Bool("reportnames", false, "report variable changes to stderr")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 || args[0] != "hook" || args[1] != "bash" {
		fmt.Fprintln(os.Stderr, "envscope: Usage: envscope [-c config] [-reportnames] hook bash")
		os.Exit(1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envscope: error getting home dir: %v\n", err)
		os.Exit(1)
	}

	configPath := *configFlag
	if configPath == "" {
		configPath = filepath.Join(homeDir, ".config", "envscope", "main.conf")
	}

	zones, allVars, err := parseConfig(configPath, homeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "envscope: error parsing config: %v\n", err)
		os.Exit(1)
	}

	generateBash(zones, allVars, *reportFlag)
}

// resolveZonePath resolves a path for a zone definition from the config file.
// Paths starting with "/" are treated as absolute. All other paths are
// considered relative to the user's home directory.
func resolveZonePath(path, homeDir string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return filepath.Join(homeDir, path)
}

// parseConfig reads the envscope configuration, constructs Zone definitions,
// and builds the parent-child hierarchy between them.
func parseConfig(configPath, homeDir string) ([]Zone, []string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	var zones []Zone
	var currentPath string
	var currentVars []EnvVar
	var allVars []string
	seenVars := make(map[string]bool)

	lineNum := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if currentPath != "" {
				if err := parseVarLine(trimmed, homeDir, &currentVars, &allVars, seenVars); err != nil {
					return nil, nil, fmt.Errorf("line %d: %w", lineNum, err)
				}
			} else {
				return nil, nil, fmt.Errorf("line %d: variable definition without a preceding zone path: %q", lineNum, trimmed)
			}
		} else {
			if currentPath != "" && len(currentVars) > 0 {
				zones = append(zones, Zone{Path: currentPath, Vars: currentVars})
			}
			currentPath = resolveZonePath(trimmed, homeDir)
			currentVars = []EnvVar{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if currentPath != "" && len(currentVars) > 0 {
		zones = append(zones, Zone{Path: currentPath, Vars: currentVars})
	}

	// Sort by path length to make parent-finding efficient and correct.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) < len(zones[j].Path)
	})

	// Assign IDs and establish parent-child relationships.
	for i := range zones {
		zones[i].ID = fmt.Sprintf("zone_%d", i)
		bestParentIdx := -1
		// Find the longest path among preceding zones that is a prefix.
		for j := 0; j < i; j++ {
			if strings.HasPrefix(zones[i].Path, zones[j].Path+"/") {
				if bestParentIdx == -1 || len(zones[j].Path) > len(zones[bestParentIdx].Path) {
					bestParentIdx = j
				}
			}
		}
		if bestParentIdx != -1 {
			zones[i].ParentID = zones[bestParentIdx].ID
		}
	}

	return zones, allVars, nil
}

// parseVarLine extracts a single variable's configurations, parsing names, plain text strings,
// and dynamic commands safely, including cache directives from comments.
func parseVarLine(line, homeDir string, currentVars *[]EnvVar, allVars *[]string, seenVars map[string]bool) error {
	origLine := line
	prepend := false
	if strings.HasPrefix(line, "+") {
		prepend = true
		line = line[1:]
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid variable definition (missing '='): %q", origLine)
	}

	name := strings.TrimSpace(parts[0])
	if !isValidVarName(name) {
		return fmt.Errorf("invalid variable name: %q", origLine)
	}

	valWithComment := parts[1]
	val := valWithComment
	cache := false

	if commentIndex := strings.Index(valWithComment, "#"); commentIndex > -1 {
		commentPart := strings.TrimSpace(valWithComment[commentIndex+1:])
		if commentPart == "cache" {
			cache = true
			val = valWithComment[:commentIndex]
		}
	}

	val = strings.TrimSpace(val)

	if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
		return fmt.Errorf("complex shell syntax in double quotes is not supported yet: %q", origLine)
	}

	var isDynamic bool
	var processedVal string

	if strings.HasPrefix(val, "$(") && strings.HasSuffix(val, ")") {
		isDynamic = true
		processedVal = val[2 : len(val)-1]
	} else {
		isDynamic = false
		processedVal = expandTilde(val, homeDir, name == "PATH")
	}

	*currentVars = append(*currentVars, EnvVar{
		Name:      name,
		Value:     processedVal,
		Prepend:   prepend,
		IsPath:    name == "PATH",
		IsDynamic: isDynamic,
		Cache:     cache,
	})

	if !seenVars[name] {
		seenVars[name] = true
		*allVars = append(*allVars, name)
	}

	return nil
}

// isValidVarName checks if a string is a valid POSIX/Bash environment variable name.
func isValidVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !isAlphaOrUnderscore(r) {
			return false
		}
		if i > 0 && !isAlphaNumOrUnderscore(r) {
			return false
		}
	}
	return true
}

func isAlphaOrUnderscore(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isAlphaNumOrUnderscore(r rune) bool {
	return isAlphaOrUnderscore(r) || (r >= '0' && r <= '9')
}

// expandTilde performs shell-like tilde expansion for a variable value.
// It expands a leading tilde (~) or tilde-slash (~/) to the user's home directory.
// If isPath is true, it also expands tildes that immediately follow a colon (:)
// to support PATH-style lists.
func expandTilde(val, homeDir string, isPath bool) string {
	expand := func(s string) string {
		if s == "~" {
			return homeDir
		}
		if strings.HasPrefix(s, "~/") {
			return homeDir + s[1:]
		}
		return s
	}

	if isPath {
		parts := strings.Split(val, ":")
		for i, p := range parts {
			parts[i] = expand(p)
		}
		return strings.Join(parts, ":")
	}

	return expand(val)
}

// generateBash drives the construction of the Bash shell hook script output.
func generateBash(zones []Zone, allVars []string, report bool) {
	var builder strings.Builder

	generateBashHeader(&builder)
	generateSaveFunction(&builder, allVars)
	generateRestoreFunction(&builder, allVars, report)
	generateParentMap(&builder, zones)
	generateApplyOneZoneFunction(&builder, zones, report)
	generateApplyStackFunction(&builder)
	generateHookFunction(&builder, zones, allVars)

	fmt.Print(builder.String())
}

// generateBashHeader sets up initial runtime states resilient against `set -u`.
func generateBashHeader(builder *strings.Builder) {
	builder.WriteString("__ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n\n")
}

// generateSaveFunction creates a Bash function to store the original environment
// state before any modifications are applied.
func generateSaveFunction(builder *strings.Builder, allVars []string) {
	builder.WriteString("__envscope_save_outer() {\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf(`  if [[ -n "${%s+x}" ]]; then
    __ENVSCP_OUTERHAD_%s=1
    __ENVSCP_OUTER_%s="$%s"
  else
    __ENVSCP_OUTERHAD_%s=0
  fi
`, v, v, v, v, v))
	}
	builder.WriteString("}\n\n")
}

// generateRestoreFunction creates a Bash function that reverts variables to their
// original "outer" state, optionally reporting changes to stderr.
func generateRestoreFunction(builder *strings.Builder, allVars []string, report bool) {
	builder.WriteString("__envscope_restore_outer() {\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf(`  if [[ "${%s:-}" == "${__ENVSCP_LAST_%s:-}" ]]; then
    if [[ ${__ENVSCP_OUTERHAD_%s:-0} -eq 1 ]]; then
      export %s="${__ENVSCP_OUTER_%s:-}"
    else
      unset %s
`, v, v, v, v, v, v))
		if report {
			builder.WriteString(fmt.Sprintf("      echo \"envscope: removed %s\" >&2\n", v))
		}
		builder.WriteString("    fi\n  fi\n")
	}
	builder.WriteString("}\n\n")
}

// generateParentMap defines a Bash associative array to represent the zone hierarchy.
func generateParentMap(builder *strings.Builder, zones []Zone) {
	builder.WriteString("declare -A __ENVSCP_PARENT=(\n")
	for _, z := range zones {
		if z.ParentID != "" {
			builder.WriteString(fmt.Sprintf("  [%s]=\"%s\"\n", z.ID, z.ParentID))
		}
	}
	builder.WriteString(")\n\n")
}

// generateApplyOneZoneFunction constructs the bash `case` structure for applying
// the variables of a single zone, with optional reporting of added variables.
func generateApplyOneZoneFunction(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("__envscope_apply_one_zone() {\n")
	builder.WriteString("  local zone=\"$1\"\n")
	builder.WriteString("  case \"$zone\" in\n")
	for _, z := range zones {
		builder.WriteString(fmt.Sprintf("    %s)\n", z.ID))
		for _, ev := range z.Vars {
			valueExpression := generateValueExpression(builder, z.ID, ev)
			if ev.Prepend {
				sep := ""
				if ev.IsPath {
					sep = ":"
				}
				builder.WriteString(fmt.Sprintf("      export %s=%s%s\"${%s:-}\"\n", ev.Name, valueExpression, sep, ev.Name))
			} else {
				builder.WriteString(fmt.Sprintf("      export %s=%s\n", ev.Name, valueExpression))
			}
			if report {
				builder.WriteString(fmt.Sprintf("      echo \"envscope: added %s\" >&2\n", ev.Name))
			}
		}
		builder.WriteString("      ;;\n")
	}
	builder.WriteString("  esac\n")
	builder.WriteString("}\n\n")
}

// escapeSingleQuotes implements safe string enclosure for Bash by replacing
// any single quotes with an escaped version and wrapping the result.
func escapeSingleQuotes(s string) string {
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return fmt.Sprintf("'%s'", escaped)
}

// generateValueExpression determines how a variable value should be expressed in Bash.
// Plain text is strictly single-quoted to prevent unintended shell evaluation.
// Dynamic commands are safely delivered via $(eval '...').
func generateValueExpression(builder *strings.Builder, zoneID string, ev EnvVar) string {
	escapedVal := escapeSingleQuotes(ev.Value)
	var expr string
	if ev.IsDynamic {
		expr = fmt.Sprintf("$(eval %s)", escapedVal)
	} else {
		expr = escapedVal
	}

	if ev.IsDynamic && ev.Cache {
		cacheVar := fmt.Sprintf("__ENVSCP_CACHE_%s_%s", zoneID, ev.Name)
		builder.WriteString(fmt.Sprintf("      if [[ -z \"${%s:-}\" ]]; then\n", cacheVar))
		builder.WriteString(fmt.Sprintf("        %s=%s\n", cacheVar, expr))
		builder.WriteString("      fi\n")
		return fmt.Sprintf("\"${%s}\"", cacheVar)
	}
	return expr
}

// generateApplyStackFunction creates a Bash function that applies all variables
// from a zone's ancestors down to the zone itself.
func generateApplyStackFunction(builder *strings.Builder) {
	builder.WriteString(`__envscope_apply_stack() {
  local zone_id="$1"
  local stack=()
  while [[ -n "$zone_id" && "$zone_id" != "NONE" ]]; do
    stack=("$zone_id" "${stack[@]}")
    zone_id="${__ENVSCP_PARENT[$zone_id]:-NONE}"
  done
  for z in "${stack[@]}"; do
    __envscope_apply_one_zone "$z"
  done
}

`)
}

// generateHookFunction produces the runtime prompt trigger evaluation loop
// implementing longest-match nested path sorting priority.
func generateHookFunction(builder *strings.Builder, zones []Zone, allVars []string) {
	// Sort longest paths first to give deepest nested folders priority.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) > len(zones[j].Path)
	})

	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  case \"$PWD\" in\n")
	for _, z := range zones {
		builder.WriteString(fmt.Sprintf("    \"%s\" | \"%s/\"* ) target_zone=\"%s\" ;;\n", z.Path, z.Path, z.ID))
	}
	builder.WriteString("  esac\n\n")

	// Prepares the snippet that records the final state of all managed variables.
	var lastVarTracker strings.Builder
	for _, v := range allVars {
		lastVarTracker.WriteString(fmt.Sprintf("  __ENVSCP_LAST_%s=\"${%s:-}\"\n", v, v))
	}

	// Evaluates the current zone versus the known state, calling out to save/restore/apply logic.
	builder.WriteString(fmt.Sprintf(`  if [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
      __envscope_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
        __envscope_save_outer
      fi
      __envscope_apply_stack "$target_zone"
%s
    fi
    __ENVSCP_ZONE="$target_zone"
  fi
}

# Attach to PROMPT_COMMAND using '|| true' to bypass 'set -e' if declare fails.
if [[ ! "${PROMPT_COMMAND:-}" =~ __envscope_hook ]] && [[ "${PROMPT_COMMAND[*]:-}" != *__envscope_hook* ]]; then
  if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null || true)" =~ "declare -a" ]]; then
    PROMPT_COMMAND+=("__envscope_hook")
  else
    PROMPT_COMMAND="${PROMPT_COMMAND:+${PROMPT_COMMAND}; }__envscope_hook"
  fi
fi
`, lastVarTracker.String()))
}
