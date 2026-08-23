# The device model label is decorative; behaviour branches only on capabilities

The connect handshake gains a human-readable model label (e.g. "Echo Show 8"),
baked into each board's binary as a constant. Nothing in the code ever branches
on it — it exists only so the dashboard can show a human which device is which.
All actual behaviour stays driven by capability negotiation.

## Why

This is the project's existing doctrine made explicit for a second device:
deciding what to do from "what model I think this is" reintroduces exactly the
version/model-sniffing the capability system was built to avoid, and it gets dev
builds and variants wrong. Recording it here stops a future contributor from
"helpfully" adding `if model == "Echo Show 8"` logic — that would be a bug, not
an optimisation. If a behaviour needs to differ, add or omit a capability
instead.
