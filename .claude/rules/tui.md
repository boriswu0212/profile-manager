---
paths:
  - "internal/tui/**"
---

# TUI: layout and scroll invariants

- **One source of truth for page capacity.** `modelRowsAvail()` /
  `profileRowsAvail()` must be used by BOTH the render functions and every
  `clampScroll` call. If render shows fewer rows than the clamp assumes, the
  cursor scrolls off-screen (this bug shipped once). When adding a line to a
  pane (header, section, indicator), update the corresponding `*RowsAvail`
  function in the same change.
- **Boxes have `Padding(0, 1)`.** Content passed into a box rendered with
  `.Width(w)` must be at most `w-2` columns wide, or lipgloss word-wraps the
  overflow onto extra lines (a wrapped model label reads as a phantom list
  entry). `View()` already passes the padding-adjusted width to the render
  functions — keep it that way.
- **Regression tests drive the real event loop.** Construct a `model` value,
  feed it `tea.KeyMsg`s through `Update`, and assert on ANSI-stripped
  `View()` output (see `app_test.go`). Any scroll/layout/keybinding change
  needs a test in this style; verify the test fails against the old behavior
  before trusting it.
- `m.profiles` shares its backing array with `m.cfg.Profiles` — writes
  through `&m.profiles[i]` are intentionally visible to `m.cfg.Save`
  (that is how set-default-model persists).
