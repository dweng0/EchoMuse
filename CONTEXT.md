# EchoMuse — Echo Show 8 Port

The language for adding **Echo Show 8** support to EchoMuse, a project that
turns retired Amazon Echo hardware into fully-local Home Assistant voice
satellites. This context covers the terms specific to supporting a second
device alongside the original Echo Dot.

## Language

**Board**:
A specific physical Echo device the firmware is built for. Each Board compiles
to its own binary, selected at build time.

**Biscuit**:
The Board for the Amazon Echo Dot 2nd Gen — the only device supported today.
_Avoid_: "the Dot" (colloquial; use for prose, not as the Board name)

**Hoya**:
The Board being added — an Amazon Echo Show 8 (1st gen) running LineageOS.
_Avoid_: "the Show", "Echo 8"

**Core**:
The board-agnostic firmware — wake-word pipeline, beamformer, echo canceller,
networking, server logic. Shared by every Board unchanged. ("The brain.")
_Avoid_: "shared code", "common"

**Bindings**:
The board-specific firmware that talks to real hardware — microphone, speaker,
buttons, LEDs. Each Board provides its own, satisfying the same Go interfaces.
("The hands.")

**Capability**:
A string a Board announces on connect to declare a feature it implements (e.g.
`mic`, `speaker`, `buttons`). The controller enables features per-Capability; an
absent Capability shows the control disabled-with-reason, never broken.

**Borrowed Screen**:
The Hoya's display is not owned by EchoMuse. The user's own software owns it at
rest (e.g. a silent visual kiosk — weather, cameras); EchoMuse only draws on it
transiently, during a voice turn. Contrast the Biscuit's LED ring, which
EchoMuse owns outright.

**Owned Audio**:
The mic and speaker, by contrast, ARE owned by EchoMuse outright on the Hoya —
it seizes the audio hardware exclusively, exactly as the Biscuit does, so no
other app can make sound while it runs. The deliberate asymmetry with the
Borrowed Screen: EchoMuse is the audio, and only a guest on the display.

**Voice Turn Overlay**:
A minimal status indicator (a listening/thinking edge bar, not a rich UI)
EchoMuse draws on the Borrowed Screen during a voice exchange. Driven by the
same turn-state signal as the Biscuit's LED scenes, so it persists across a
multi-turn conversation — re-showing when the assistant re-opens the mic for a
follow-up — and collapses only when the whole exchange ends.

**Hardware Map**:
The documented inventory of how to drive each piece of Hoya hardware — which
ALSA card and device the mic and speaker live on, their sample formats and
channel counts, and the input-device paths for buttons and the mute switch.
Produced by discovery on the real device before any Bindings are written; every
Binding depends on it. Analogous to what SETUP.md records for the Biscuit.

**Provisioning Script**:
The single script that installs EchoMuse onto a Hoya — pushes the binary and
TLS credentials and sets it to auto-start. It is the one source of truth for
provisioning: usable standalone from day one, and later wrapped by the
dashboard wizard, which just runs it and streams its console output to the user
rather than reimplementing the steps.

## Relationships

- A **Board** provides one set of **Bindings** and reuses the shared **Core**.
- A **Board** announces a set of **Capabilities** derived from the **Bindings**
  it actually provides.
- **Hoya** omits the LED Capability and instead (later) adds a **Voice Turn
  Overlay** drawn on the **Borrowed Screen**.

## Flagged ambiguities

- "Echo 8" was used to mean the **Hoya** (Echo Show 8 1st gen) — resolved.
- "Copy-paste the Dot" was used to describe the port — resolved: we reuse the
  **Core** untouched and write fresh **Bindings**; nothing is copied line-for-line.
