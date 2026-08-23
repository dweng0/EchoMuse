# One binary per board, selected by build tag

Adding the Echo Show 8 (Hoya) means a second set of hardware bindings alongside
the Echo Dot 2nd Gen (Biscuit). We build a separate firmware binary per board,
gated by a Go build tag (mirroring the existing `server` tag with a `hoya` tag),
rather than shipping one universal binary that detects the hardware at runtime.

## Why

The bindings are welded to specific chips, ALSA device numbers and input-device
paths, and they use cgo against board-specific native libraries. A build tag
keeps each board's low-level, non-portable code out of the other board's binary
entirely — no runtime probing that can guess wrong, no dead cgo linked into a
device that can't use it. The shared Core stays in one tree; only the hands
differ per tag.

## Consequences

- Each board is released as its own binary; the release/build pipeline gains a
  second target.
- Behaviour is never chosen by "which board am I" at runtime — that stays the
  job of capability negotiation. The build tag only decides which bindings are
  compiled in. See ADR-0003.
