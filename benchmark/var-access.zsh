#!/bin/zsh

N=40000
VAL="abcdefghij1234567890" # 20 chars

# Pre-generate variable names
var_names=()
for i in {1..40}; do var_names+=("var_$i"); done

echo "--- Zsh Benchmark (N=$N, 40 items) ---"

# --- CASE 0: Direct Variable Access (Hardcoded) ---
echo "Case 0: Direct Variable Access (40 hardcoded vars)"
time (
    for ((i=0; i<N; i++)); do
        # Write
        v1=$VAL; v2=$VAL; v3=$VAL; v4=$VAL; v5=$VAL; v6=$VAL; v7=$VAL; v8=$VAL; v9=$VAL; v10=$VAL
        v11=$VAL; v12=$VAL; v13=$VAL; v14=$VAL; v15=$VAL; v16=$VAL; v17=$VAL; v18=$VAL; v19=$VAL; v20=$VAL
        v21=$VAL; v22=$VAL; v23=$VAL; v24=$VAL; v25=$VAL; v26=$VAL; v27=$VAL; v28=$VAL; v29=$VAL; v30=$VAL
        v31=$VAL; v32=$VAL; v33=$VAL; v34=$VAL; v35=$VAL; v36=$VAL; v37=$VAL; v38=$VAL; v39=$VAL; v40=$VAL
        
        # Read
        t=$v1; t=$v2; t=$v3; t=$v4; t=$v5; t=$v6; t=$v7; t=$v8; t=$v9; t=$v10
        t=$v11; t=$v12; t=$v13; t=$v14; t=$v15; t=$v16; t=$v17; t=$v18; t=$v19; t=$v20
        t=$v21; t=$v22; t=$v23; t=$v24; t=$v25; t=$v26; t=$v27; t=$v28; t=$v29; t=$v30
        t=$v31; t=$v32; t=$v33; t=$v34; t=$v35; t=$v36; t=$v37; t=$v38; t=$v39; t=$v40
    done
)

# --- CASE 1: 40 Variables ---
echo "Case 1: Dynamic Variable Access (Indirection)"
time (
    for ((i=0; i<N; i++)); do
        # Write operations
        for name in $var_names; do
            typeset -g "$name=$VAL"
        done
        # Read operations
        for name in $var_names; do
            tmp="${(P)name}"
        done
    done
)

# --- CASE 2: Array elements ---
echo -e "\nCase 2: Array Access"
set -A arr
time (
    for ((i=0; i<N; i++)); do
        # Write operations
        for j in {1..40}; do
            arr[$j]="$VAL"
        done
        # Read operations
        for j in {1..40}; do
            tmp="${arr[$j]}"
        done
    done
)
