# v2.2.2 (6/11/2026)
- Graph Paper grid added: repeating square-cell overlay for freeform layout (per-tab, like other grids).
- Shell asset cache busting: CSS/JS URLs and the service worker now use a deploy revision so grid and shortcut fixes load without a hard refresh.
- Press `s` to toggle select and hand pan canvas mode (same as the top-right hand/select button).
- Arrow keys pan the canvas camera (same pan math as wheel and trackpad scrolling).

# v2.2.1 (6/9/2026)
- Importance grid added: four-column layout with Low, Medium, High, and Critical categories.

# v2.2.0 (6/9/2026)
- Language selector in Settings with partial locales for Spanish, French, German, and Chinese.
- Settings modal, Shortcuts modal, import/export toasts, and Mermaid import chrome now use the i18n layer.
- Selected language is stored in browser-local storage; English remains the default fallback.

# v2.1.2 (6/8/2026)
- Keyboard shortcut help moved from the bottom-left corner into a Shortcuts modal, opened by a keyboard icon.
- Removed the Hide Instructions setting; shortcut reference is always available from the icon.
- Fixed canvas stacking so notes render above the shortcut trigger and trash controls.

# v2.1.1 (6/7/2026)
- Import supported Mermaid flowcharts and graphs from Settings (Import & Export) to create Postbaby notes and edges.
- Mermaid adapter parses a supported flowchart/graph subset (TD, TB, LR) with clear import status, errors, and warnings.

# v2.1.0 (6/6/2026)
- Infinite canvas with pan/zoom camera (replaces the old workspace scroll model).
- Select and pan (hand) modes, with move/selection toggle and grab-handle polish.
- On-screen camera controls, optional hide for the camera remote, and keyboard shortcuts to recenter the view.
- Grid layer stacks under notes so the canvas reads correctly at any zoom.

# v2.0.0 (5/16/2026) (first unified release)
- Unified the browser-local and self-hosted codepaths into one repo.
- Self-hosted Docker is now a Go + SQLite deployment on port `8080` with required `/app/data` persistence and first-run `/setup`.
- This is **not** a drop-in upgrade from the old static/Caddy image.
- Old browser-local `v1.35` `localStorage["items"]` data is **not** auto-migrated.
- Existing users should read `README.md` and `docs/NAS.md` before upgrading.
- After upgrading, hard refresh once if the app looks broken. Installed PWA users may need to close/reopen the app or clear site data.

# 1.62.2 (4/2/2025)
- Added delete hotkey to delete multiple selected items & right click to delete multple selected items.

# 1.62.1 (4/2/2025)
- Added current time indicator line on week grid.

# 1.62.0 (3/26/2025)
- Added week grid

# 1.61.2 (1/27/2025)
- Bug fixed which would resize and elongate items intermittently.

# 1.60.1 (12/15/2024)
- Notes will resize accordingly in Calendar grid.

# 1.60 (12/5/2024)
- Ability to drag multiple notes at the same time by holding CTRL.

# 1.53 (11/27/2024)
- Ability to save & load data file added.

# 1.52 (11/26/2024)
- Now & Later Grid added.
- Default color on note creation option added.

# 1.51 (11/14/2024)
- implemented current month calendar grid.

# 1.5 (11/12/2024)
- introduced grids
- implemented the following grids: Kanban, Eisenhower Matrix, Priority Matrix, S.M.A.R.T Goals, and SWOT Analysis.

# v1.41 (11/8/2024)
- added options (accessible by clicking/tapping on trash) 
- corporate mode (hides logo and changes toilet-paper to trash can)

# v1.4 (11/1/2024)
- ability to add tabs added.

# v1.35 (10/29/2024)
- refactoring of code, removal of non-functioning toilet-roll in mobile view.

# v1.3 (10/28/2024)
- single-tap color change & items creation at position of click implemented.

# v1.2 (10/28/2024)
- initial load default items added.

# v1.1 (10/25/2024)
- introduced mobile view.

# v1.0 (10/24/2024)
- basic functionality.
