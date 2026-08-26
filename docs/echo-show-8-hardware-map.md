# Hardware Map — Echo Show 8 (1st gen, 2019)

The inventory every device **Binding** for the Show 8 depends on — the analogue
of what `SETUP.md` records for the Echo Dot. Produced by discovery on a real
device (issue #1). Terms in **bold** are defined in [CONTEXT.md](../CONTEXT.md).

**Device:** Amazon Echo Show 8, 1st gen (2019). SoC **MediaTek MT8163**
(quad-core Cortex-A53). Two 2″ stereo speakers; mic array; 1MP camera with a
physical shutter. LineageOS port codename **`crown`** (XDA unofficial).

> **Codename note.** The LineageOS port and every device prop report **`crown`**
> (`ro.product.device=crown`, `lineage_crown`). An earlier draft of the plan and
> ADR-0001 named the build tag `hoya` — a guess, and the wrong one (`hoya` is
> Amazon's codename for the Echo Show *5*). Reconciled: the build tag is
> **`crown`** throughout, matching what the platform calls itself.

## Platform

| Property | Value | Source |
|---|---|---|
| CPU ABI | `armeabi-v7a` (32-bit; `abilist=armeabi-v7a,armeabi`) | `getprop ro.product.cpu.abi` |
| Go target | `GOARCH=arm GOARM=7` (no 64-bit userspace) | derived |
| Android API | **30** (Android 11) | `getprop ro.build.version.sdk` |
| ROM | LineageOS `18.1-20260624-UNOFFICIAL-crown` | `getprop ro.lineage.version` |
| SELinux | **Permissive** | `getenforce`, `ro.boot.selinux` |
| Root | userdebug — `adb root` restarts adbd as uid 0. **No `su` binary, no Magisk.** | `adb root`; `id` → `uid=0 ... context=u:r:su:s0` |

The Dot boots with a bogus pre-NTP clock; note the Show's `adb root` gives a
real root shell without Magisk, so provisioning does not need a `su` wrapper.

## Audio

Single sound card **`card0`** (`mtsndcard`, `mt-snd-card`). MediaTek exposes
many front-end PCMs (`/proc/asound/pcm`); the app-facing endpoints are the
MultiMedia1 front-ends, with the physical codecs reached via `tinymix` routing.

| Role | ALSA | Physical codec | Format |
|---|---|---|---|
| **Mic (capture)** | `card0,device22` (`TLV320AIC3101 Capture`) | **2× TLV320AIC3101** (I²C `0-0018`/`0-0019`, 4 mics) | **6ch, 16000 Hz, `S24_3LE`** — HAL-observed; the DAI accepts *only* this |
| **Speaker (playback)** | `card0,device0` (`MultiMedia1_Playback`) | **RT5616** (PCM `00-23`), driving the ext amp | stereo expected; confirm at first playback |

Tooling present on-device: **`tinymix`, `tinycap`**. **`tinyplay` and
`tinypcminfo` are absent** — the speaker binding cannot be smoke-tested with
stock tools; push a `tinyplay`/test binary, or verify via the binding itself
(issue #5). `tinycap` **cannot capture the mic**: it can't request `S24_3LE`
(3-byte-packed 24-bit), the only format `device22` accepts. A real capture
needs the GoTinyAlsa binding requesting `S24_3LE` directly (issue #6).

### The mic is a known-UNSOLVED bring-up problem — RE-SCOPE issue #6

The MVP milestone (#6) is **not a straightforward binding**. Findings, from
observing the working LineageOS HAL plus the `amazon-oss` kernel source:

- **The HAL captures the mic on `card0,device22` directly** (the AIC3101
  front-end, `amzn-mt-spi-pcm`), at **6ch / 16 kHz / `S24_3LE`**. Verified by
  diffing `tinymix` around a live Recorder-app capture (`/proc/asound/.../
  pcm22c/.../status` = `RUNNING`, `hw_params` = those values).
- **No `tinymix` routing is involved** — the mixer diff during a live HAL
  capture was **empty**. Opening the PCM powers the codec. My earlier attempts
  on `device1` (internal PMIC ADC) and `device15` (I2S0AWB) were both dead ends
  by design; both return digital zeros.
- **Hardware:** 4 mics across two AIC3101 dies (`0x18`/`0x19`), differential
  `DIF1` inputs, TDM. The 6 channels = 4 mics + 2 spare/reference slots.
  `crown_defconfig`: `CONFIG_SND_SOC_4_MICS=y`.
- **Gain is NOT the fix.** MICPGA is already maxed (`ADC_A/B MICPGA Volume Ctrl`
  range `0→80`, sitting at `80`); digital volume `88/104`. XDA `crown` users who
  maxed gain still get "faint static, no voice."
- **Original (partly wrong) root-cause guess:** stock uses a smart-mic DSP
  firmware (`i2s_to_spi_6ch_v183.bin`, RT551X) that clocks the AIC3101 TDM and
  does beamforming/AGC, and LineageOS was assumed to fall back to a *generic MTK
  HAL*, leaving the array near-silent. **The source investigation below
  corrects this** — the LineageOS `crown` mic is substantially brought up, and
  the likely cause of "quiet" is narrower and testable.

### Source investigation (2026-08-24) — the mic is more brought-up than it looked

Reading the LineageOS `crown` sources ([kernel](https://github.com/amazon-oss/android_kernel_amazon_mt8163),
[device tree](https://github.com/amazon-oss/android_device_amazon_crown),
[org](https://github.com/amazon-oss)) changes the picture:

- **The HAL is Amazon's *closed blob*, not a generic MTK HAL.** Per the kernel
  maintainer (commit `479f405a`): *"The proper fix would be in the HAL, but it
  is a closed blob and we have neither its sources nor whatever modifications
  Amazon applied on top."* So LineageOS ships Amazon's audio HAL, and the mic
  path is already wired, not bypassed.
- **The kernel already brings up the capture path.** Relevant commits in
  `android_kernel_amazon_mt8163`
  (`sound/soc/codecs/tlv320aic3101.c`,
  `sound/soc/mediatek/mt_soc_audio_8163_amzn/amzn-spi-pcm/amzn-mt-spi-pcm.c`):
  - `479f405a` — **"Mix both mics for mono capture"**: the HAL reads a single
    channel of the AIC3101 SPI stream, so only one mic is captured; the kernel
    averages ch0+ch1 as a hack, **enabled per-board via `amzn,mic-downmix`**.
  - `61ff588b` — re-work probe of the secondary ADC (the second AIC3101 die).
  - `c72fb388` — AIC3101 external reset control.
  - `73cbfee1` — **imports the DSP firmware** (`i2s_to_spi_4ch_v208.bin`).
  - `d9dad4db` — correct supported sample rates.
- **`crown` exposes a normal mic to Android.** `android_device_amazon_crown`'s
  `configs/audio_policy_configuration.xml` declares a `Built-In Mic`
  (`AUDIO_DEVICE_IN_BUILTIN_MIC`) with a `primary input` at **16 kHz,
  `AUDIO_CHANNEL_IN_MONO`/`STEREO`**. So the ordinary record path
  (`AudioRecord`, or ALSA `plughw`) *does* capture — no codec bring-up needed to
  get bytes.
- **The "quiet" was directly addressed upstream.** `android_device_amazon_crown`
  commit `7fd09fa5` (2026-04-18) is literally **"crown: Boost built-in mic
  capture gain"**, and the XDA changelog lists *"microphone volume was fixed"*
  (alongside a Wi-Fi-disconnect fix). So a *current* LineageOS `crown` build
  likely captures at a usable level already — test with the latest build before
  assuming quiet.
- **Retracted lead — `amzn,mic-downmix` is NOT crown's lever.** That hack
  averages **ch0+ch1** and is enabled for `checkers`/`cronos`, whose mics sit on
  those two channels. Crown has **4 mics across 2 dies**, a different layout, and
  its maintainer fixed the level via the gain boost above, not downmix — so do
  not treat "missing `amzn,mic-downmix` on `crown`" as the fix.
- **Still unproven:** boosted gain ≠ guaranteed reliable *across-room* wake
  (gain also lifts the noise floor), and the once-per-boot silent-capture bug's
  status is unconfirmed. Both need the on-hardware SNR/wake measurement below.
- **Known LineageOS bug:** capture works for the *first* recording after boot,
  then goes silent until `audioserver` restarts (reported on XDA). A satellite
  that opens capture once and *holds* it may sidestep this — or may trip it on
  reconnect. Testable.

**Re-scoped consequence for issue #6:** the MVP path is **capture normally
(`plughw`/`AudioRecord`) → stream to the controller → controller-side wake
word** — no on-device codec work required. The upstream mic-gain fix means a
current build may already be usable, so the go/no-go is simply a one-off level/SNR
**measurement on a recent LineageOS `crown` build**. Speaker (#5) is unaffected
and looks easy — sequence #5 before #6, and treat #6 as "measure on a current
build," downgraded from "high-risk spike." A TDM bring-up from scratch is not on
the table.

### On-hardware capture (2026-08-26) — the go/no-go measurement, done

Ran `device/tools/capture_mics -card 0 -device 22 -channels 6 5` (built with the
new per-flag card/device/channel args, previously hardcoded to `biscuit`'s
0/24/9ch) against a real `crown` unit over `adb`, as root. **Opens and streams
clean at 6ch/16kHz/`S24_3LE` — no ALSA errors, no digital zeros.** (One
blocker on the way: `/dev/snd/pcmC0D22c` is `system:audio`-owned and `adb
shell` starts as uid 2000 `shell`, not root — needs `adb root` first, same as
any other privileged ALSA node on this platform.)

Per-channel RMS/peak over a 4.64s capture, with music playing in the room
(not a quiet-room SNR test, just a "does it capture real audio" check):

| Channel | RMS (dBFS) | Peak (dBFS) |
|---|---|---|
| ch0 | −32.3 | −17.9 |
| ch1 | −32.5 | −18.1 |
| ch2 | −29.4 | −13.3 |
| ch3 | −30.1 | −14.3 |
| ch4 | −46.1 | −30.1 |
| ch5 | −47.0 | −29.8 |

ch0–3 (the 4 real capsules) run hot with real, unclipped signal; ch4–5 (the
spare/reference slots) sit ~15dB quieter, consistent with them not being live
mic channels. **This confirms the interface-doc reframe: capture is a config
change, not a bring-up problem** — the upstream gain fix is doing its job.

**Not yet measured: quiet-room, speech-level, across-room SNR** — this was a
loud-room sanity check, not the wake-reliability test. The once-per-boot
silent-capture bug (line 117 above) also remains unconfirmed either way; this
run was a single 5s capture, not a reconnect-cycle test.

**References**

- XDA `crown` thread (canonical discussion, incl. mic reports):
  <https://xdaforums.com/t/rom-unofficial-11-crown-lineageos-18-1-for-the-amazon-echo-show-8-2019.4766709/>
- Kernel: [`amazon-oss/android_kernel_amazon_mt8163`](https://github.com/amazon-oss/android_kernel_amazon_mt8163)
- Device tree: [`amazon-oss/android_device_amazon_crown`](https://github.com/amazon-oss/android_device_amazon_crown),
  [`android_device_amazon_mt8163-common`](https://github.com/amazon-oss/android_device_amazon_mt8163-common) (branch `lineage-18.1`)
- Vendor/HAL blobs: `android_vendor_amazon_crown`, `android_vendor_amazon_mt8163-common`
- TI codec datasheet (AIC3101, up to 59.5 dB analog gain / AGC): <https://www.ti.com/product/TLV320AIC3101>
- FCC teardown (internal photos — mic-array / board layout): <https://fccid.io/2ARO5-7879>

### Speaker amp is already enabled
`Ext_Speaker_Amp_Switch = On`, `Ext_Amp_Gain = 6dB` at rest — the playback amp
path is live; the playback route needs less coaxing than capture.

## Inputs (buttons, mute, switches)

All live-confirmed with `getevent -lt` while pressing each control:

| Control | Device | Event | Code | Behaviour |
|---|---|---|---|---|
| Volume up | `/dev/input/event6` (`gpio-keys`) | `EV_KEY` | `KEY_VOLUMEUP` | momentary |
| Volume down | `/dev/input/event6` (`gpio-keys`) | `EV_KEY` | `KEY_VOLUMEDOWN` | momentary |
| Mic / action button | `/dev/input/event0` (`gating`, amazon-specific) | `EV_KEY` | `KEY_POWER` | momentary DOWN/UP — firmware toggles mute in software |
| Camera shutter | `/dev/input/event6` (`gpio-keys`) | `EV_SW` | `SW_CAMERA_LENS_COVER` | **latching** (1=closed, 0=open) |

Other input nodes (not used by bindings, recorded to avoid opening the wrong
one — resolve by NAME, never by number):

- `event2` `fts_ts` — **touchscreen** (`BTN_TOUCH` + `ABS_MT_*`). The Dot's
  CLAUDE.md warns event2 is the volume button on biscuit; here it is the screen.
- `event3` `ACCDET` — headphone-jack accessory detect.
- `event5` `m_alsps_input` — ambient-light + proximity sensor (an ALS exists,
  like the Dot's `tsl2540` — resolve by name if ever used).
- `event1` `mtk-kpd`, `event4` `hwmdata` — no useful keys.

## Auto-start

- **No Magisk / no `service.d`.** Root is the userdebug `adb root` path.
- Init service dirs exist and are the autostart surface: `/system/etc/init/`
  and `/vendor/etc/init/` (writable via `adb remount` on userdebug; there is a
  leftover `/vendor/etc/init/amazon_init.rc`). LineageOS `addon.d`
  (`50-lineage.sh`) survives OTA.
- `/data/local/tmp` is shell-writable; `/data/local` is root-only.

**Plan for issue #3:** drop an init `.rc` service into `/system/etc/init/` (or
`/vendor/etc/init/`) that execs the pushed binary, after `adb remount`. No
`su`/Magisk wrapper needed.

## Discovery commands (reproduce this map)

```sh
adb root
adb shell getprop ro.product.cpu.abi                 # armeabi-v7a
adb shell getprop ro.build.version.sdk               # 30
cat /proc/asound/cards ; cat /proc/asound/pcm        # card0 mtsndcard; PCM list
adb shell tinymix                                     # mixer controls / routing
adb shell 'tinycap /data/local/tmp/m.wav -D 0 -d 1 -c 2 -r 48000 -b 16 -T 3'
adb shell getevent -lt                                # press each control to map it
```
