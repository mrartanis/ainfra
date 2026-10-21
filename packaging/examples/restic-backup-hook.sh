#!/bin/sh
# Operator-invoked wrapper around an actual restic backup, never snapshots.
# Usage: restic-backup-hook.sh OUTPUT_JSON CHECK_ID /absolute/path/restic backup ARGS...
set -eu
umask 077
[ "$#" -ge 4 ] || exit 64
output=$1
check_id=$2
shift 2
case "$output" in /*) ;; *) exit 64 ;; esac
case "$check_id" in ''|*[!a-zA-Z0-9_.-]*) exit 64 ;; esac
case "$1" in /*/restic) ;; *) exit 64 ;; esac
[ "$2" = backup ] || exit 64
attempt=$(date -u +%Y-%m-%dT%H:%M:%SZ)
result=0
"$@" || result=$?
observed=$(date -u +%Y-%m-%dT%H:%M:%SZ)
status=error
success=null
if [ "$result" -eq 0 ]; then
    status=success
    success="\"$observed\""
fi
check_status=error
if [ "$result" -eq 0 ]; then check_status=ok; fi
temporary=$(mktemp "$output.pending.XXXXXX")
trap 'test ! -e "$temporary" || unlink "$temporary"' EXIT
printf '{"schema_version":1,"check_id":"%s","kind":"backup","observed_at":"%s","status":"%s","data":{"last_attempt_at":"%s","last_attempt_status":"%s","last_success_at":%s}}\n' "$check_id" "$observed" "$check_status" "$attempt" "$status" "$success" > "$temporary"
chmod 0640 "$temporary"
mv -- "$temporary" "$output"
exit "$result"
