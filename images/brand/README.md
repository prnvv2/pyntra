# Pyntra brand

**Pyntra — Security as a Service**

Design canvas: https://claude.ai/artifact/3QQTCw6WEbbnodimRKuxnY

## Symbol

A geometric **P**: a red stem, an open bowl that stops short of the stem, and a square aperture dot in the counter.

| File | Use |
|---|---|
| `pyntra-symbol.svg` | Symbol on light backgrounds |
| `pyntra-symbol-dark.svg` | Symbol on dark backgrounds |
| `pyntra-app-icon.svg` | App icon / favicon tile (white on red) |
| `pyntra-lockup.svg` | Symbol + wordmark, light backgrounds |
| `pyntra-lockup-dark.svg` | Symbol + wordmark, dark backgrounds |

Below 32 px, thicken the strokes (≈9 at 32 px, ≈11 at 16 px in the 64-unit viewBox) or use the app icon.

## Color

| Token | Hex | Role |
|---|---|---|
| Brand red | `#DC2626` | Stem + dot, primary buttons |
| Red (dark mode) | `#F0575C` | Bowl on dark backgrounds |
| Ink | `#0A0A0B` | Bowl on light, dark backgrounds |
| White | `#FFFFFF` | Light backgrounds, text on red |

Matches `web/static/css/tokens.css` (`--brand-600`, dark `--primary`).

## Type

- Wordmark and headings: **Sora** 600, tracking −0.035em
- Labels and code: **IBM Plex Mono**

The lockup SVGs use live `<text>`; outline the wordmark before using them where Sora isn't installed.
