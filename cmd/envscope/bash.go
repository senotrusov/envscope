// Copyright 2026 Stanislav Senotrusov
//
// This work is dual-licensed under the Apache License, Version 2.0
// and the MIT License. Refer to the LICENSE file in the top-level directory
// for the full license terms.
//
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
	"strings"
)

func generateBash(builder *strings.Builder, zones []Zone, allVars []string, report bool) {
	generateBashHeader(builder)
	generateVarsArrayBash(builder, allVars)
	generateSaveFunctionBash(builder)
	generateRestoreFunctionBash(builder, report)
	generateParentMapBash(builder, zones)
	generateApplyOneZoneFunctionBash(builder, zones, report)
	generateApplyStackFunctionBash(builder)
	generateHookFunctionBash(builder, zones)
}

func generateBashHeader(builder *strings.Builder) {
	builder.WriteString("__ENVSCP_ZONE=${__ENVSCP_ZONE:-\"NONE\"}\n")
	builder.WriteString("declare -a __ENVSCP_C 2>/dev/null || true\n\n")
}

func generateVarsArrayBash(builder *strings.Builder, allVars []string) {
	builder.WriteString("declare -a __ENVSCP_VARS=(\n")
	for _, v := range allVars {
		builder.WriteString(fmt.Sprintf("  \"%s\"\n", v))
	}
	builder.WriteString(")\n\n")
}

func generateSaveFunctionBash(builder *strings.Builder) {
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

func generateRestoreFunctionBash(builder *strings.Builder, report bool) {
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

func generateParentMapBash(builder *strings.Builder, zones []Zone) {
	builder.WriteString("declare -A __ENVSCP_PARENT=(\n")
	for _, z := range getSortedZonesByID(zones) {
		if z.ParentID != -1 {
			builder.WriteString(fmt.Sprintf("  [%s]=\"%s\"\n", z.Name(), z.ParentName()))
		}
	}
	builder.WriteString(")\n\n")
}

func generateApplyOneZoneFunctionBash(builder *strings.Builder, zones []Zone, report bool) {
	builder.WriteString("__envscope_apply_one_zone() {\n")
	builder.WriteString("  local zone=\"$1\"\n")
	builder.WriteString("  case \"$zone\" in\n")
	for _, z := range getSortedZonesByID(zones) {
		builder.WriteString(fmt.Sprintf("    %s)\n", z.Name()))
		for _, ev := range z.Vars {
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
				expr = fmt.Sprintf("\"${__ENVSCP_C[%d]}\"", ev.CacheIndex)
			}

			if ev.Prepend {
				sep := ""
				if ev.IsPath {
					sep = ":"
				}
				builder.WriteString(fmt.Sprintf("      export %s=%s%s\"${%s:-}\"\n", ev.Name, expr, sep, ev.Name))
			} else {
				builder.WriteString(fmt.Sprintf("      export %s=%s\n", ev.Name, expr))
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

func generateApplyStackFunctionBash(builder *strings.Builder) {
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

func generateHookFunctionBash(builder *strings.Builder, zones []Zone) {
	builder.WriteString("__envscope_hook() {\n")
	builder.WriteString("  local target_zone=\"NONE\"\n")
	builder.WriteString("  local current_pwd=\"${PWD:-}\"\n")
	builder.WriteString("  current_pwd=\"${current_pwd%/}/\"\n")
	builder.WriteString("  case \"$current_pwd\" in\n")
	for _, z := range zones {
		pattern := formatZonePattern(z.Path)
		builder.WriteString(fmt.Sprintf("    %s ) target_zone=\"%s\" ;;\n", pattern, z.Name()))
	}
	builder.WriteString("  esac\n\n")

	var lastVarTracker strings.Builder
	lastVarTracker.WriteString(`      __ENVSCP_L=()
      for i in "${!__ENVSCP_VARS[@]}"; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${!v:-}"
      done`)

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
