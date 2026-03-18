package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// EnvVar represents a parsed environment variable configuration.
type EnvVar struct {
	Name       string
	Value      string
	Prepend    bool
	IsPath     bool
	IsDynamic  bool
	Cache      bool
	CacheIndex int
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
	reportFlag := flag.Bool("reportvars", false, "report variable changes to stderr")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 || args[0] != "hook" || args[1] != "bash" {
		fmt.Fprintln(os.Stderr, "envscope: Usage: envscope [-c config] [-reportvars] hook bash")
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
	var currentPaths []string
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
			if len(currentPaths) > 0 {
				if err := parseVarLine(trimmed, homeDir, &currentVars, &allVars, seenVars); err != nil {
					return nil, nil, fmt.Errorf("line %d: %w", lineNum, err)
				}
			} else {
				return nil, nil, fmt.Errorf("line %d: variable definition without a preceding zone path: %q", lineNum, trimmed)
			}
		} else {
			if len(currentPaths) > 0 && len(currentVars) > 0 {
				for _, p := range currentPaths {
					zones = append(zones, Zone{Path: p, Vars: currentVars})
				}
				currentPaths = nil
				currentVars = nil
			}
			currentPaths = append(currentPaths, resolveZonePath(trimmed, homeDir))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if len(currentPaths) > 0 && len(currentVars) > 0 {
		for _, p := range currentPaths {
			zones = append(zones, Zone{Path: p, Vars: currentVars})
		}
	}

	// Sort by path length to make parent-finding evaluate longest potentials first.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) < len(zones[j].Path)
	})

	// Assign IDs.
	for i := range zones {
		zones[i].ID = fmt.Sprintf("zone_%d", i)
	}

	// Establish parent-child relationships.
	for i := range zones {
		bestParentIdx := -1
		for j := range zones {
			if i == j {
				continue
			}
			if isSubPath(zones[j].Path, zones[i].Path) {
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

// isSubPath checks if the child path is logically nested under the parent path.
// It supports wildcard '*' characters in the parent path to allow for complex subsets.
func isSubPath(parent, child string) bool {
	if parent == "/" {
		return true
	}
	parentPath := parent
	if !strings.HasSuffix(parentPath, "/") {
		parentPath += "/"
	}
	childPath := child
	if !strings.HasSuffix(childPath, "/") {
		childPath += "/"
	}

	// A zone is not considered a parent of an identical zone.
	if parentPath == childPath {
		return false
	}

	parts := strings.Split(parentPath, "*")
	var rxParts []string
	for _, p := range parts {
		rxParts = append(rxParts, regexp.QuoteMeta(p))
	}
	rxStr := "^" + strings.Join(rxParts, ".*")
	matched, _ := regexp.MatchString(rxStr, childPath)
	return matched
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
	generateVarsArray(&builder, allVars)
	generateSaveFunction(&builder)
	generateRestoreFunction(&builder, report)
	generateParentMap(&builder, zones)

	// Pre-calculate deterministic integer indices for all dynamic cached variables.
	cacheCounter := 0
	for i := range zones {
		for j := range zones[i].Vars {
			if zones[i].Vars[j].Cache {
				zones[i].Vars[j].CacheIndex = cacheCounter
				cacheCounter++
			}
		}
	}

	generateApplyOneZoneFunction(&builder, zones, report)
	generateApplyStackFunction(&builder)
	generateHookFunction(&builder, zones)

	fmt.Print(builder.String())
}

// generateBashHeader sets up initial runtime states resilient against `set -u`
// and creates the global indexed cache array.
func generateBashHeader(builder *strings.Builder) {
	builder.WriteString("__ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n")
	builder.WriteString("declare -a __ENVSCP_C 2>/dev/null || true\n\n")
}

// generateVarsArray defines a global array of all managed variable names
// to allow compact iteration via variable indirection.
func generateVarsArray(builder *strings.Builder, allVars []string) {
	builder.WriteString("declare -a __ENVSCP_VARS=(\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf("  \"%s\"\n", v))
	}
	builder.WriteString(")\n\n")
}

// generateSaveFunction creates a Bash function to compactly store the original
// environment state before any modifications are applied using indexed arrays.
func generateSaveFunction(builder *strings.Builder) {
	builder.WriteString(`__envscope_save_outer() {
  __ENVSCP_H=()
  __ENVSCP_O=()
  for i in "${!__ENVSCP_VARS[@]}"; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ -n "${!v+x}" ]]; then
      __ENVSCP_H[$i]=1
      __ENVSCP_O[$i]="${!v}"
    else
      __ENVSCP_H[$i]=0
    fi
  done
}

`)
}

// generateRestoreFunction creates a Bash function that reverts variables to their
// original "outer" state from indexed arrays, optionally reporting changes.
func generateRestoreFunction(builder *strings.Builder, report bool) {
	builder.WriteString(`__envscope_restore_outer() {
  for i in "${!__ENVSCP_VARS[@]}"; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ "${!v:-}" == "${__ENVSCP_L[$i]:-}" ]]; then
      if [[ ${__ENVSCP_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVSCP_O[$i]:-}"
      else
`)
	if report {
		builder.WriteString(`        if [[ -n "${!v+x}" ]]; then
          unset "$v"
          echo "envscope: removed $v" >&2
        fi
`)
	} else {
		builder.WriteString(`        unset "$v"
`)
	}
	builder.WriteString(`      fi
    fi
  done
}

`)
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
			valueExpression := generateValueExpression(builder, ev)
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
func generateValueExpression(builder *strings.Builder, ev EnvVar) string {
	escapedVal := escapeSingleQuotes(ev.Value)
	var expr string
	if ev.IsDynamic {
		expr = fmt.Sprintf("$(eval %s)", escapedVal)
	} else {
		expr = escapedVal
	}

	if ev.IsDynamic && ev.Cache {
		builder.WriteString(fmt.Sprintf("      if [[ -z \"${__ENVSCP_C[%d]:-}\" ]]; then\n", ev.CacheIndex))
		builder.WriteString(fmt.Sprintf("        __ENVSCP_C[%d]=%s\n", ev.CacheIndex, expr))
		builder.WriteString("      fi\n")
		return fmt.Sprintf("\"${__ENVSCP_C[%d]}\"", ev.CacheIndex)
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

// formatZonePattern converts a zone path into a safely quoted Bash case pattern,
// appending wildcards where necessary to match current and nested directories.
func formatZonePattern(path string) string {
	matchPath := path
	if !strings.HasSuffix(matchPath, "/") {
		matchPath += "/"
	}

	parts := strings.Split(matchPath, "*")
	var res strings.Builder
	for i, p := range parts {
		if i > 0 {
			res.WriteString("*")
		}
		if p != "" {
			res.WriteString(escapeSingleQuotes(p))
		}
	}

	res.WriteString("*")
	return res.String()
}

// generateHookFunction produces the runtime prompt trigger evaluation loop
// implementing longest-match nested path sorting priority.
func generateHookFunction(builder *strings.Builder, zones []Zone) {
	// Sort longest paths first to give deepest nested folders priority.
	sort.Slice(zones, func(i, j int) bool {
		return len(zones[i].Path) > len(zones[j].Path)
	})

	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  local current_pwd=\"${PWD:-}\"\n")
	builder.WriteString("  current_pwd=\"${current_pwd%/}/\"\n")
	builder.WriteString("  case \"$current_pwd\" in\n")
	for _, z := range zones {
		pattern := formatZonePattern(z.Path)
		builder.WriteString(fmt.Sprintf("    %s ) target_zone=\"%s\" ;;\n", pattern, z.ID))
	}
	builder.WriteString("  esac\n\n")

	// Prepares the snippet that records the final state of all managed variables compactly.
	var lastVarTracker strings.Builder
	lastVarTracker.WriteString(`      __ENVSCP_L=()
      for i in "${!__ENVSCP_VARS[@]}"; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${!v:-}"
      done`)

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
    else
      unset __ENVSCP_L __ENVSCP_O __ENVSCP_H
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
