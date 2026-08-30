#!/bin/bash
# provision_crown.sh — installs EchoMuse onto a crown (Echo Show 8) device
# and wires up autostart, in one pass. Single script (not a manual step
# sequence) so a future dashboard wizard can shell out to it and stream
# its stdout — every step prints one line before/after for that reason.
#
# Does NOT mint TLS credentials itself: that needs an authenticated call
# to the controller's admin API, which this script has no browser session
# for. Without -c/-t the device links plain (ws://) — the documented
# rollout fallback — upgradeable later via the dashboard's "Secure link".
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
# echomuse_crown.rc / raw init `service` is NOT used here — verified dead
# on real hardware: init refuses to exec untrusted /data binaries on a
# non-Magisk device regardless of SELinux mode. crown_launcher.apk
# (BOOT_COMPLETED -> foreground service -> exec) is the only autostart
# path that survives a power cycle on this device.
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

# /dev/snd/* and /dev/input/* ship 0660 (system:audio / root:input) on
# stock crown — fine for system processes, but the daemon runs as the
# launcher APK's own sandboxed uid (u0_a###), which is in neither group.
# Symptom is silent: ServerService's ProcessBuilder starts the daemon fine,
# it logs "Failed to initialize Microphone: ... permission denied" and
# exits 1, START_STICKY loops it forever, and the launcher itself looks
# perfectly healthy in `ps` throughout. Found the hard way on a second
# crown unit 2026-08-30 — the first had this patched by hand and nobody
# had folded it back in here yet (docs/echo-show-8-journal.md 2026-08-26).
#
# crown has no separate vendor partition (/vendor -> /system/vendor), so
# the live rule is /system/etc/ueventd.rc. RECORD_AUDIO does not help —
# that only gates the Binder AudioRecord path, and this daemon opens
# /dev/snd directly (same as biscuit's tinyalsa approach). A priv-app
# allowlist doesn't help either — device-node gid membership for real
# system audio processes comes from a build-time AID, not anything a
# pushed APK can acquire after install. Patching the node perms is the
# only lever that actually works from here.
#
# Idempotent: skip if a previous run (or a hand-patch, like the first
# crown) already did this — repatching every provision would be harmless
# but the reboot it needs is not free.
CURRENT_SND_PERM="$(adb shell stat -c '%a' /dev/snd/pcmC0D22c 2>/dev/null | tr -d '\r')"
if [ "$CURRENT_SND_PERM" = "666" ]; then
    echo "-- /dev/snd already 0666 — ueventd patch not needed"
else
    echo "-- patching /system/etc/ueventd.rc: /dev/snd/*, /dev/input/* 0660 -> 0666"
    adb remount >/dev/null
    UEVENTD_TMP="$(mktemp)"
    adb pull /system/etc/ueventd.rc "$UEVENTD_TMP" >/dev/null
    # Keep one pristine backup on-device, first patch only — never overwrite
    # it on a later re-run, or it stops being the thing to diff against.
    adb shell "[ -f /system/etc/ueventd.rc.echomuse-orig ] || cp /system/etc/ueventd.rc /system/etc/ueventd.rc.echomuse-orig"
    sed -i \
        -e 's|^/dev/input/\*\(\s*\)0660|/dev/input/*\10666|' \
        -e 's|^/dev/snd/\*\(\s*\)0660|/dev/snd/*\10666|' \
        "$UEVENTD_TMP"
    adb push "$UEVENTD_TMP" /system/etc/ueventd.rc >/dev/null
    adb shell "chmod 644 /system/etc/ueventd.rc && chown root:root /system/etc/ueventd.rc"
    rm -f "$UEVENTD_TMP"
    echo "   done — rebooting to let ueventd re-apply node perms"
    adb reboot
    adb wait-for-device
    # Same race as the adb root above, just longer: a full boot, not adbd.
    sleep 8
    adb wait-for-device
    adb root >/dev/null
    sleep 1
    adb wait-for-device
fi

echo "-- installing binary -> /data/local/bin/server"
adb shell "mkdir -p /data/local/bin" >/dev/null
adb push "$BINARY" /data/local/tmp/server.new >/dev/null
# Move into place rather than pushing directly to the final path: a
# service could be actively exec'ing the old file mid-push otherwise
# (ETXTBSY, or a half-written binary exec'd on the next crash-restart).
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

echo "-- granting SYSTEM_ALERT_WINDOW (status strip overlay)"
# SYSTEM_ALERT_WINDOW is a special permission — Android grants it only via
# a manual Settings visit or an appops call from a shell that already
# holds it (ours does). Without it StatusOverlay just logs and no-ops
# (see StatusOverlay.addView's catch) rather than crashing the service.
adb shell appops set "$LAUNCHER_PKG" SYSTEM_ALERT_WINDOW allow >/dev/null
echo "   done"

echo "-- pre-creating the daemon log file (writable by the app's own uid)"
# ServerService execs the daemon as its own sandboxed app uid, never
# root, and redirects stdout here via ProcessBuilder — but a freshly-reset
# device's /data/local/tmp lacks directory WRITE for that uid, so
# ProcessBuilder.start() throws and is caught silently (no crash, no log;
# `ps` shows the launcher alive but the daemon never started). Pre-creating
# the file needs only directory search/execute, which the app already has.
adb shell "touch /data/local/tmp/echomuse.log && chmod 666 /data/local/tmp/echomuse.log"
echo "   done"

echo "-- clearing Android's 'stopped' state + starting now"
# A freshly-installed app that has never been run sits in the "stopped"
# package state, and Android withholds implicit broadcasts — including
# BOOT_COMPLETED — from stopped apps. Plain `am start-service` hits
# "Error: app is in background uid null" on such a device (Android 8+
# background-start restriction); `am start-foreground-service` clears the
# stopped flag permanently AND starts the service immediately.
adb shell am start-foreground-service -n "$LAUNCHER_SVC" >/dev/null
echo "   started — tail /data/local/tmp/echomuse.log on-device, or:"
echo "     adb shell tail -f /data/local/tmp/echomuse.log"

echo "== provision_crown: done =="
