#!/bin/bash
# provision_crown.sh — installs EchoMuse onto a crown (Echo Show 8) device
# and wires up autostart, in one pass.
#
# Deliberately a single script rather than a manual step sequence, so a
# future dashboard wizard (docs/echo-show-8-plan.md, "Later / nice-to-have")
# can shell out to this exact thing and stream its stdout, the same relation
# the existing wizard has to device_payloads/start_server.sh for biscuit.
# Every step below prints one line before and one after, on purpose, for
# that streaming case — don't collapse them into a single silent block.
#
# What it does NOT do, and why: mint TLS credentials itself. That needs an
# authenticated call to the controller's admin API
# (POST /api/provision/tls_credentials), and this script has no browser
# session to make it with. The existing wizard makes that call from the
# dashboard's own authenticated JS and would hand this script the resulting
# ca.pem/token as local files — so that's the contract here too. Run
# without -c/-t, the device links up plain (ws://), which is the documented
# rollout fallback (root CLAUDE.md, "Link auth & TLS") — upgrade later with
# the dashboard's "Secure link" action once the device is already connected
# and approved, exactly as an already-fielded device would.
#
# Usage:
#   provision_crown.sh -b build/crown [-a crown_launcher.apk] [-c ca.pem -t token]
#
#   -b  path to the crown server binary (required)
#   -a  path to crown_launcher.apk (default: ../crown_launcher/build/crown_launcher.apk
#       next to this script — the output of device/crown_launcher/build.sh)
#   -c  path to ca.pem, from POST /api/provision/tls_credentials (optional)
#   -t  path to a file containing just the token, same call (optional; -c
#       and -t must be given together or not at all)
#
# echomuse_crown.rc / raw init `service` is NOT used here — verified dead on
# real hardware 2026-08-26 (see docs/echo-show-8-hardware-map.md): init
# refuses to exec untrusted /data binaries on a non-Magisk device regardless
# of SELinux mode. crown_launcher.apk (BOOT_COMPLETED receiver -> foreground
# service -> exec) is the only autostart path that actually survives a
# power cycle on this device.
set -euo pipefail

BINARY=""
APK=""
CA_PEM=""
TOKEN_FILE=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APK_DEFAULT="$SCRIPT_DIR/../crown_launcher/build/crown_launcher.apk"
LAUNCHER_PKG="com.echomuse.crownlauncher"
LAUNCHER_SVC="$LAUNCHER_PKG/.ServerService"

while getopts "b:a:c:t:h" opt; do
    case "$opt" in
        b) BINARY="$OPTARG" ;;
        a) APK="$OPTARG" ;;
        c) CA_PEM="$OPTARG" ;;
        t) TOKEN_FILE="$OPTARG" ;;
        h) sed -n '2,26p' "$0"; exit 0 ;;
        *) echo "usage: $0 -b <binary> [-a launcher.apk] [-c ca.pem -t token]" >&2; exit 1 ;;
    esac
done

if [ -z "$BINARY" ] || [ ! -f "$BINARY" ]; then
    echo "✗ -b <binary> is required and must exist (got: '${BINARY}')" >&2
    exit 1
fi
if { [ -n "$CA_PEM" ] && [ -z "$TOKEN_FILE" ]; } || { [ -z "$CA_PEM" ] && [ -n "$TOKEN_FILE" ]; }; then
    echo "✗ -c and -t must be given together, or neither" >&2
    exit 1
fi

APK="${APK:-$APK_DEFAULT}"
if [ ! -f "$APK" ]; then
    echo "✗ launcher APK not found at $APK — build it first: device/crown_launcher/build.sh (or pass -a)" >&2
    exit 1
fi

echo "== provision_crown: $(adb get-serialno 2>/dev/null || echo 'no device') =="

echo "-- adb root"
adb root >/dev/null
# adb root restarts adbd; the next command can race it coming back.
sleep 1
adb wait-for-device

echo "-- installing binary -> /data/local/bin/server"
adb shell "mkdir -p /data/local/bin" >/dev/null
adb push "$BINARY" /data/local/tmp/server.new >/dev/null
# Move into place rather than pushing directly to the final path: a
# service could be actively exec'ing the old file mid-push otherwise
# (ETXTBSY, or worse, a half-written binary getting exec'd on the next
# crash-restart).
adb shell "mv -f /data/local/tmp/server.new /data/local/bin/server && chmod 755 /data/local/bin/server"
echo "   done"

if [ -n "$CA_PEM" ]; then
    echo "-- installing TLS credentials -> /data/local/etc/echomuse"
    adb shell "mkdir -p /data/local/etc/echomuse" >/dev/null
    adb push "$CA_PEM" /data/local/etc/echomuse/ca.pem >/dev/null
    adb push "$TOKEN_FILE" /data/local/etc/echomuse/token >/dev/null
    echo "   done (device will dial wss:// once the controller advertises tls_port)"
else
    echo "-- skipping TLS credentials (none given — device will link plain ws://)"
fi

echo "-- installing crown_launcher.apk (autostart)"
adb install -r -g "$APK" >/dev/null
echo "   done"

echo "-- clearing Android's 'stopped' state + starting now"
# A freshly-installed app that has never been run sits in the "stopped"
# package state, and Android withholds implicit broadcasts — including
# BOOT_COMPLETED — from stopped apps (measured live 2026-08-26: installed,
# rebooted, nothing started). `am start-service` both clears that state
# permanently and starts the service immediately, so this one command
# replaces the old raw `setsid exec` line AND fixes the thing that made the
# APK approach not work on a first install.
adb shell am start-service -n "$LAUNCHER_SVC" >/dev/null
echo "   started — tail /data/local/tmp/echomuse.log on-device, or:"
echo "     adb shell tail -f /data/local/tmp/echomuse.log"

echo "== provision_crown: done =="
