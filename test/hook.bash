__ENVSCP_ZONE=${__ENVSCP_ZONE:-"NONE"}
declare -a __ENVSCP_C 2>/dev/null || true

declare -a __ENVSCP_VARS=(
  "TESTROOT"
  "LOCALVAR"
  "DATE_VAR"
  "DATE_VAR_CACHED"
  "QUOTED_VAR"
  "SPACED_VAR"
  "TILDE_VAR"
  "TILDE_VAR_EXACT"
  "TILDE_VAR_MID"
  "TILDE_PATH_NOT_PATH"
  "PATH"
  "MULTI_VAR"
  "WILDCARD_VAR"
  "ROOT_VAR"
)

__envscope_save_outer() {
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

__envscope_restore_outer() {
  for i in "${!__ENVSCP_VARS[@]}"; do
    local v="${__ENVSCP_VARS[$i]}"
    if [[ "${!v:-}" == "${__ENVSCP_L[$i]:-}" ]]; then
      if [[ ${__ENVSCP_H[$i]:-0} -eq 1 ]]; then
        export "$v"="${__ENVSCP_O[$i]:-}"
      else
        unset "$v"
      fi
    fi
  done
}

declare -A __ENVSCP_PARENT=(
  [zone_1]="zone_0"
  [zone_2]="zone_1"
  [zone_3]="zone_1"
  [zone_4]="zone_2"
  [zone_5]="zone_1"
  [zone_6]="zone_1"
  [zone_7]="zone_1"
)

__envscope_apply_one_zone() {
  local zone="$1"
  case "$zone" in
    zone_0)
      export ROOT_VAR='root-zone'
      ;;
    zone_1)
      export TESTROOT='testroot-value'
      export LOCALVAR='test'
      export DATE_VAR=$(eval 'echo $RANDOM')
      if [[ -z "${__ENVSCP_C[0]:-}" ]]; then
        __ENVSCP_C[0]=$(eval 'echo $RANDOM')
      fi
      export DATE_VAR_CACHED="${__ENVSCP_C[0]}"
      export QUOTED_VAR='val'\''withquote'
      export SPACED_VAR='val  spaced'
      export TILDE_VAR='/home/user/foo'
      export TILDE_VAR_EXACT='/home/foo'
      export TILDE_VAR_MID='a~/foo'
      export TILDE_PATH_NOT_PATH=':/bin:~/foo'
      ;;
    zone_2)
      export PATH='/home/user/prefix-that-does-not-exist':"${PATH:-}"
      export TESTROOT='now-with-prefix-'"${TESTROOT:-}"
      export LOCALVAR='test-foo'
      ;;
    zone_3)
      export PATH='/home/user/bin:/usr/bin:/home/user/local/bin:/home/foo'
      ;;
    zone_4)
      export LOCALVAR='test-foo-bar'
      ;;
    zone_5)
      export MULTI_VAR='applied-to-both'
      ;;
    zone_6)
      export MULTI_VAR='applied-to-both'
      ;;
    zone_7)
      export WILDCARD_VAR='matched'
      ;;
  esac
}

__envscope_apply_stack() {
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

__envscope_hook() {
  local target_zone="NONE"
  local current_pwd="${PWD:-}"
  current_pwd="${current_pwd%/}/"
  case "$current_pwd" in
    '/home/user/test/wildcard/'*'/bar/'* ) target_zone="zone_7" ;;
    '/home/user/test/foo/bar/'* ) target_zone="zone_4" ;;
    '/home/user/test/multi-1/'* ) target_zone="zone_5" ;;
    '/home/user/test/multi-2/'* ) target_zone="zone_6" ;;
    '/home/user/test/tilde/'* ) target_zone="zone_3" ;;
    '/home/user/test/foo/'* ) target_zone="zone_2" ;;
    '/home/user/test/'* ) target_zone="zone_1" ;;
    '/'* ) target_zone="zone_0" ;;
  esac

  if [[ "$target_zone" != "${__ENVSCP_ZONE:-NONE}" ]]; then
    if [[ "${__ENVSCP_ZONE:-NONE}" != "NONE" ]]; then
      __envscope_restore_outer
    fi
    if [[ "$target_zone" != "NONE" ]]; then
      if [[ "${__ENVSCP_ZONE:-NONE}" == "NONE" ]]; then
        __envscope_save_outer
      fi
      __envscope_apply_stack "$target_zone"
      __ENVSCP_L=()
      for i in "${!__ENVSCP_VARS[@]}"; do
        local v="${__ENVSCP_VARS[$i]}"
        __ENVSCP_L[$i]="${!v:-}"
      done
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
