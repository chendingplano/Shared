#!/usr/bin/env bash
set -e
: "${SYSTEM_MONITOR_LOG_ROOT:=$HOME/Apps/ChenWebLog/system-monitor}"
: "${NATS_LOG_FILE:=$SYSTEM_MONITOR_LOG_ROOT/nats.log}"
mkdir -p "$(dirname "$NATS_LOG_FILE")"
exec mise run nets-server-run-userpass > >(tee -a "$NATS_LOG_FILE") 2>&1
