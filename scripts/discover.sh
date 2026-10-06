#!/usr/bin/env bash
#
# discover.sh — find Gooxi BMCs in a subnet.
#
# A host is reported as a Gooxi BMC when:
#   1. its HTTPS port accepts a TCP connection, AND
#   2. POST /api/session answers with a JSON body (Gooxi's API signature).
#
# Usage:
#   ./scripts/discover.sh [CIDR]          # default: 192.168.0.0/24
#
# Environment overrides:
#   PORT             HTTPS port to probe         (default 443)
#   CONNECT_TIMEOUT  TCP connect timeout, sec    (default 2)
#   HTTP_TIMEOUT     HTTP request timeout, sec   (default 3)
#   PARALLELISM      concurrent probes           (default 50)

set -uo pipefail

SUBNET="${1:-192.168.0.0/24}"
PORT="${PORT:-443}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-5}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-10}"
PARALLELISM="${PARALLELISM:-50}"

command -v curl >/dev/null 2>&1 || { echo "error: curl is required" >&2; exit 1; }

# Expand a CIDR (or a single IP) into one IP per line. Pure bash, no nmap needed.
expand_cidr() {
  local cidr="$1" base prefix net int_ip mask start count n ip a b c d
  base="${cidr%/*}"
  prefix="${cidr#*/}"
  [ "$prefix" = "$cidr" ] && prefix=32
  net=$(( 32 - prefix ))
  IFS=. read -r a b c d <<< "$base"
  int_ip=$(( (a << 24) + (b << 16) + (c << 8) + d ))
  mask=$(( (0xFFFFFFFF << net) & 0xFFFFFFFF ))
  start=$(( int_ip & mask ))
  count=$(( 1 << net ))
  for (( n = 0; n < count; n++ )); do
    ip=$(( (start + n) & 0xFFFFFFFF ))
    printf '%d.%d.%d.%d\n' \
      $(( (ip >> 24) & 255 )) $(( (ip >> 16) & 255 )) \
      $(( (ip >> 8) & 255 ))  $(( ip & 255 ))
  done
}

# Probe one host: TCP-connect to $PORT, then look for the Gooxi JSON signature.
check_host() {
  local ip="$1" resp
  # 1) Is the HTTPS port open?
  timeout "$CONNECT_TIMEOUT" bash -c 'echo >/dev/tcp/'"$ip"'/'"$PORT" </dev/null 2>/dev/null || return 0
  # 2) Does /api/session answer with a JSON body?
  resp=$(curl -sk --max-time "$HTTP_TIMEOUT" </dev/null \
        -X POST "https://$ip/api/session" \
        -H 'Content-Type: application/x-www-form-urlencoded' \
        -d 'username=probe&password=probe' 2>/dev/null) || true
  [[ "$resp" =~ ^[[:space:]]*[{}] ]] && printf '%s\n' "$ip"
}
export -f check_host
export PORT CONNECT_TIMEOUT HTTP_TIMEOUT

echo "Scanning $SUBNET for Gooxi BMCs (port $PORT, parallelism $PARALLELISM)..." >&2

found="$(expand_cidr "$SUBNET" \
  | xargs -P "$PARALLELISM" -I{} bash -c 'check_host "$@"' _ {} \
  | sort -u -t. -k1,1n -k2,2n -k3,3n -k4,4n)"

if [ -n "$found" ]; then
  printf '%s\n' "$found"
  echo "Found $(printf '%s\n' "$found" | wc -l | tr -d ' ') Gooxi BMC(s) in $SUBNET" >&2
else
  echo "No Gooxi BMCs found in $SUBNET" >&2
fi
