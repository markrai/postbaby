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
