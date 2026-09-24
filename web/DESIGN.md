# Pyntra Console — Design System

Presentation layer for the Pyntra web console (`web/`). Vanilla HTML/CSS/JS, no build step.
Every endpoint, request/response shape, SSE stream and WebSocket message is unchanged; this
document covers only how the UI looks and is organized.

## Principles

- **Calm, dense, trustworthy.** Neutral slate surfaces, one indigo primary, no gradients,
  glass, neon or emoji.
- **Color carries meaning only** for severity (Critical/High/Medium/Low/Info) and run status
  (running/completed/failed/queued/paused).
- **Tokens only.** Components never contain raw colors or ad-hoc sizes; everything resolves
  to a variable in `static/css/tokens.css`.
- **Offline.** Fonts, icons and libraries are served from `static/vendor/` and `static/js/icons.js`.

## File map

| File | Role |
|---|---|
| `static/css/tokens.css` | Single source of truth: fonts, color ramps, semantic tokens (light + dark), type, spacing, radii, elevation, z-index, motion, metrics. Also keeps legacy variable aliases (`--accent-color`, `--bg-secondary`, …) so older markup keeps rendering. |
| `static/css/icons.css` | Lucide icons as CSS mask variables (`--i-wrench`, …), generated from `js/icons.js`. |
| `static/css/pages/screens.css` | Layout rules for screens without a dedicated page file (tasks detail, files, knowledge, webshell panels, …). Generated from the retired `style.css` with every color, gradient, shadow, radius and font mapped onto tokens. Loaded first so everything below wins. |
| `static/css/base.css` | Reset, typography, scrollbars, app shell (top bar, grouped sidebar, page header, breadcrumbs), filter bars, pagination, loading/empty/error states, login, context menus. |
| `static/css/components.css` | Buttons, inputs, selects, toggles, checkboxes, badges, status chips, cards, KPI stats, tables, tabs, modals, drawer, tooltip, toast, empty state, skeleton, code block, breadcrumbs, segmented control, custom select, focus rings. |
| `static/css/pages/dashboard.css` | KPI tiles, severity donut/legend, run overview, tool bars, CTA. |
| `static/css/pages/chat.css` | Conversation sidebar, message stream, live run card, tool-call timeline, composer, pickers, mentions, attachments, chat modals. |
| `static/css/pages/data.css` | Findings table + drawer, stat strips, table toolbar. |
| `static/css/pages/manage.css` | MCP tools/servers, role/skill/agent cards, webshell split view, settings, terminal. |
| `static/js/icons.js` | `pyIcon(name, {size})` → inline SVG string; `pyIconEl()` → element. 102 Lucide v0.469.0 icons (ISC). |
| `static/vendor/` | marked, DOMPurify, cytoscape, elkjs, xterm (+fit), i18next, Inter, JetBrains Mono — pinned to the versions previously loaded from CDN, with licenses. |
| *(removed)* | The previous `style.css`, `theme-pro.css` and `theme-fx.css` are no longer shipped. They remain in git history (commit `825360a`) and are the source `screens.css` was generated from. |

Load order in `templates/index.html`: `xterm.css → screens.css → tokens.css → icons.css → base.css → components.css → pages/*.css`.

## Tokens

### Color (semantic)

| Token | Use |
|---|---|
| `--surface-0` | App canvas |
| `--surface-1` | Cards, panels, header, sidebar |
| `--surface-2` | Table headers, subtle panels, footers |
| `--surface-3` | Insets, code wells, tracks |
| `--surface-hover` / `--surface-active` | Row/list hover and selected |
| `--text-1` / `--text-2` / `--text-3` | Primary / secondary / muted text |
| `--border-1` / `--border-2` | Hairline / interactive border |
| `--primary`, `--primary-hover`, `--primary-subtle`, `--primary-subtle-border` | The single accent (indigo) |
| `--success`, `--warning`, `--danger`, `--info` (+ `-bg`, `-border`) | Feedback |
| `--sev-critical … --sev-info` (+ `-bg`) | Vulnerability severity |
| `--status-running / completed / failed / queued / paused` | Run lifecycle |

Dark values apply for `html[data-theme="dark"]` and for the OS dark preference unless the user
forced light. The theme toggle persists `pyntra_theme` in `localStorage`; the default is dark.

### Typography

Inter for UI, JetBrains Mono for code, logs, IDs and tool names.
Scale: `--text-xs` 12 · `--text-sm` 14 (body) · `--text-md` 16 · `--text-lg` 20 · `--text-xl` 24 · `--text-2xl` 32.
Weights 400/500/600/700. Numbers in tables and KPIs use `font-variant-numeric: tabular-nums`.

### Spacing, shape, elevation, motion

- Spacing on a 4px grid: `--space-1` 4 … `--space-10` 64.
- Controls: `--control-height-xs/sm/-/lg` = 24/28/32/40.
- Radii: `--radius-xs` 4 · `--radius-sm` 6 (default) · `--radius-md` 8 · `--radius-lg` 12 · `--radius-pill`.
- Borders are 1px (`--border-width`).
- Shadows are very subtle (`--shadow-xs/sm/md/lg`); focus uses `--shadow-focus`.
- Z-index: `--z-base` 1 · `--z-sticky` 100 · `--z-dropdown` 400 · `--z-drawer` 800 · `--z-overlay` 1000 · `--z-modal` 1100 · `--z-popover` 1200 · `--z-toast` 1300.
- Motion: `--duration-fast/base/slow` (120/200/320ms) with `--ease-standard` and `--ease-emphasized`. All durations drop to 0 under `prefers-reduced-motion`.

## Layout rules

- **Shell:** 48px top bar (brand, API docs, theme toggle, user menu). The sidebar is grouped into
  Overview / Operations / Platform / System, and collapses to a 56px icon rail with tooltips
  (state in `localStorage.sidebarCollapsed`). The collapse control sits in the sidebar footer.
- **Page header on every screen:** breadcrumbs above the title, with actions on the right.
  The primary action is the only filled button.
- **Content:** 20/24px page padding. Sections are `surface-1` cards with a 1px border, and card
  headers are 14px semibold.
- **Tables:** sticky header, 40px rows, hover highlight, a checkbox column for bulk actions,
  sortable headers (`aria-sort`), a toolbar above that shows the selection count, and a pagination
  bar below. Row click opens a detail drawer (right side, Esc to close, focus returns to the row).
- **Forms:** labels above inputs, helper text below (`.form-hint`), required fields marked with
  `.required`, on/off settings rendered as switches.
- **States:** every list has loading (spinner or skeleton rows), empty (`.pc-empty` with an icon,
  title, text and action) and error (`.pc-empty.is-error` or `.error-message`) states.
- **Chat:**
  - User turns are right-aligned on a primary tint. Assistant turns are full-width documents.
  - Live runs show a card with a spinner, elapsed timer and a Stop button, and its state
    (running/completed/failed/cancelled) comes from `data-state`.
  - Tool calls and results are collapsible timeline cards with an icon per event type, a status
    chip and a duration.
  - The composer is two rows: the message field on top, with role, mode, attach and send below.

## Components (class reference)

`btn-primary` · `btn-secondary` · `btn-ghost` · `btn-danger` · `btn-small` · `btn-icon`
· form inputs (native elements are styled directly) · `pc-toggle` / `.checkbox-label > .modern-checkbox + .checkbox-custom`
· `badge` + `badge-{critical,high,medium,low,info,neutral,primary}` · `sev-dot`
· `status-chip` + `status-{running,completed,failed,queued,paused,cancelled}`
· `pc-card` · `pc-stat` · `pc-table` / `pc-table-wrap` / `table-toolbar` · `pc-tabs` / `pc-tab`
· `modal` / `modal-content[.modal-sm|.modal-lg|.modal-xl]` · `pc-drawer` / `pc-drawer-scrim` / `pc-dl`
· `pc-tooltip[data-tooltip]` · `pc-toast` · `pc-empty` · `pc-skeleton` · `code-block` / `pc-code`
· `pc-breadcrumbs` · `pc-segmented` / `btn-filter` · `pc-progress`.

## Accessibility

- Visible focus rings on every interactive element (`:focus-visible`).
- Sidebar items, KPI tiles, table rows and timeline headers are keyboard operable
  (Enter/Space) and carry `role`/`aria-*` attributes.
- Icons are `aria-hidden`, and icon-only buttons have `aria-label`s.
- Text and background pairs were chosen for WCAG 2.1 AA contrast in both themes.
  The muted `--text-3` is used only for secondary metadata.

## Conventions for new UI

1. Use existing tokens. Only add a token when a genuinely new role appears.
2. Put screen-specific rules in `pages/<area>.css`. Never write inline `style=""` except for
   runtime state (`display`, computed widths).
3. Use `pyIcon('name')` for icons. Never use emoji or symbol glyphs.
4. Render user-supplied text through `escapeHtml()` / `textContent`.

## Open questions

- **Version badge:** the header has a version slot, but nothing in `web/` supplies a version.
  It is hidden until the server exposes one.
- **Terminal:** `/api/terminal/ws` returns HTTP 501 on this build, so the terminal tab shows a
  connection error. The request is unchanged; this is backend behavior.
- **Alerts and confirmations:** these still use the browser's native `alert()`/`confirm()`
  dialogs. The toast and dialog components exist, but routing the ~180 alerts and 35 confirms
  through them is pending.
- **Global search** (a command palette in the top bar) was specified but is not implemented yet.
- **Provider list:** the Settings "API format" select only knows `openai`/`claude`. A provider
  from `config.yaml` (e.g. `ollama`) is now preserved as an extra option. A full list could come
  from `GET /api/config/providers`.
- `screens.css` is generated. Areas that deserve a hand-written page stylesheet next are tasks
  (batch-queue detail), file management, knowledge items/retrieval logs, and the webshell
  workspace.
