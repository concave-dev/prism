#!/usr/bin/env bash

set -euo pipefail

# Kill all prismd instances and anything bound to expected port ranges
#
# Port ranges (width=20):
# - Serf: 4200-4219 (TCP+UDP)
# - Raft: 6969-6988 (TCP)
# - gRPC: 7117-7136 (TCP)
# - API : 8008-8027 (TCP)
#
# TODO: Make base ports and range width configurable via env/flags
# TODO: Extend to detect dynamically chosen ports from logs/PIDs for greater accuracy

SERF_START=${SERF_START:-4200}
RAFT_START=${RAFT_START:-6969}
GRPC_START=${GRPC_START:-7117}
API_START=${API_START:-8008}
RANGE_WIDTH=${RANGE_WIDTH:-20}

calc_end() {
  local start=$1
  local width=$2
  echo $((start + width - 1))
}

SERF_END=$(calc_end "$SERF_START" "$RANGE_WIDTH")
RAFT_END=$(calc_end "$RAFT_START" "$RANGE_WIDTH")
GRPC_END=$(calc_end "$GRPC_START" "$RANGE_WIDTH")
API_END=$(calc_end "$API_START" "$RANGE_WIDTH")

kill_pids() {
  local pids=("$@")
  if [ ${#pids[@]} -eq 0 ]; then
    return 0
  fi
  # Unique PIDs (portable, avoids 'mapfile' not present on macOS's bash 3.2)
  local unique_pids
  unique_pids=$(printf "%s\n" "${pids[@]}" | awk 'NF' | sort -u)
  if [ -z "$unique_pids" ]; then
    return 0
  fi
  echo "Killing PIDs: $unique_pids"
  # Force kill to avoid lingering listeners
  # shellcheck disable=SC2086
  kill -9 $unique_pids 2>/dev/null || true
}

kill_port_range() {
  local label=$1
  local start=$2
  local end=$3
  echo "Checking ${label} ports: ${start}-${end}"

  # TCP listeners
  local tcp_pids=""
  if tcp_pids=$(lsof -ti tcp:${start}-${end} -sTCP:LISTEN 2>/dev/null); then
    if [ -n "${tcp_pids}" ]; then
      echo " Found TCP listeners ("${label}"): ${tcp_pids//\n/ }"
      kill_pids ${tcp_pids}
    fi
  fi

  # UDP sockets (Serf primarily, but harmless to check for all ranges)
  local udp_pids=""
  if udp_pids=$(lsof -ti udp:${start}-${end} 2>/dev/null); then
    if [ -n "${udp_pids}" ]; then
      echo " Found UDP sockets ("${label}"): ${udp_pids//\n/ }"
      kill_pids ${udp_pids}
    fi
  fi
}

echo "Killing prismd processes by name (best-effort)"
pkill -f prismd 2>/dev/null || true

# Also catch binaries that may appear as 'main' in lsof output by killing by port
kill_port_range "Serf" "$SERF_START" "$SERF_END"
kill_port_range "Raft" "$RAFT_START" "$RAFT_END"
kill_port_range "gRPC" "$GRPC_START" "$GRPC_END"
kill_port_range "API"  "$API_START"  "$API_END"

echo "All done."


