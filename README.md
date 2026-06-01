![postbabygithubtitle](https://github.com/user-attachments/assets/29979323-15ae-4324-817e-f89571606655)

a self-hosted version of [postbaby.org](https://www.postbaby.org)

This was created out of necessity for a sticky-note solution with rapid arrangement, editing, and color-coding capability, thanks to its intuitive key-bindings.

🖼️ Supported on both landscape and portrait-oriented monitors, as well as mobile devices.

> [!IMPORTANT]
> **Docker users upgrading:** this is **not** a drop-in upgrade for old Docker deployments.
> The old static/Caddy container behavior is gone.
> The container now runs the Go app on port `8080`, Compose maps `8080:8080`, `/app/data` is required for persistence, and first run uses `/setup`.
> Read [docs/NAS.md](./docs/NAS.md) before upgrading.

> [!IMPORTANT]
> **Browser-local v1.35 users upgrading:** old flat `localStorage["items"]` notes are **not** auto-migrated in this release.
> Upgrading will usually show a fresh board or default notes instead of the old `v1.35` notes.
> The old raw browser storage may still exist in the browser profile, but the current app does not surface or import it.

> [!IMPORTANT]
> **After upgrading:** if the app looks broken, hard refresh once.
> Installed PWA users should close and reopen the app, and if needed clear site data.
> Static/PWA upgrades can behave oddly when filenames and paths change.

# ⚡Install

## Browser-local / ephemeral

- Clone the repository and open `index.html` directly, or serve the repo root as static files.
- No backend, login, or server sync is required.
- Notes stay in this browser only.

## Self-hosted Docker / NAS

1. Rename `.env.example` to `.env`.
2. Run:

```powershell
docker compose up --build -d
```

3. Open [http://localhost:8080/](http://localhost:8080/).
4. On a fresh database, Postbaby redirects to `/setup`.
5. Create the bootstrap user and sign in.

The default Compose setup persists data in `./postbaby-data` on the host and mounts it to `/app/data` in the container.
For NAS and Portainer-specific setups, see [docs/NAS.md](./docs/NAS.md).


# 🧭 Modes

- `static_local`: browser-local / ephemeral behavior, no login, no server sync.
- `selfhosted_single_user`: Go backend + SQLite + `/setup` + `/login` + cookie-session sync.


# 💾 Storage

- The current app uses **IndexedDB as the primary browser-local store**.
- Current-era browser-local keys can be migrated into IndexedDB or used as fallback data if needed.
- In self-hosted mode, the browser stays local-first and syncs account-backed snapshots to the server.
- Export and import are available from Settings in the current app.

**Origin warning:** browser-local notes live in the storage bucket for the exact origin you used.
`https://postbaby.org`, `http://localhost:8080`, and `file://...` do **not** share data.
If you test locally on a different origin, do not expect notes from another origin to appear automatically.

# 🧱 Frontend Artifact

* The app serves the committed `js/script.js` file directly.
* Self-hosters do not need to build or regenerate frontend assets for normal deployment.
* Maintainer-only scripts such as `npm run build:public-js`, `npm run verify:public-js`, and `npm run test:public-js-build` are used only when updating the generated frontend artifact.

# ⌨️ Desktop Usage Instructions

- `Right-click` on blank canvas or press `n` to create a new item.
- `Right-click` on an existing item (or tab) to delete it, or drag the item to the toilet roll to delete.
- `Left-click` on an existing item (or tab) to cycle through its colors.
- `Double-click` on an existing item (or tab) to edit its text.
- Press `Delete` to remove the current multi-selection.
- Press `c` to clear all items in the active tab.
- Press `Tab` to cycle tabs and `Shift + Tab` to cycle in reverse.
- Press keys `1-9` to jump to corresponding tabs.
- Press `g` to cycle grid modes in the active tab.
- Press `CTRL + right-click` to move to the next shape.
- Press `SHIFT + CTRL + right-click` to move to the previous shape.
- Drag on empty canvas to create a multi-selection box.
- Hold `Shift` and drag from an item to draw a line.
- Hold `Ctrl` and drag from an item to draw an arrow.
- Press `CTRL + ALT + H` to re-enable first-time-run welcome notes.
- `Left-click` the toilet roll / trash icon to open settings.

# 📱 Mobile Usage Instructions

- `Long-press` to create a new item.
- `Long-press` an existing item (or tab) to delete it.
- `Double tap` to edit an existing item (or tab) text.
- `Single tap` an existing item (or tab) to change its color.
- `Single tap` the toilet roll / trash icon to open settings.
- Desktop keyboard shortcuts still work if a physical keyboard is connected.

# 🐳 Upgrade Notes

- Old static/Caddy Docker deployments are not interchangeable with this build.
- The app now ships as one codebase with a Go backend for self-hosted sync mode.
- The self-hosted container listens on `8080`, not `80`.
- Persistent data must be mounted to `/app/data`.
- First-run account setup happens at `/setup`.
- Existing users should read [docs/NAS.md](./docs/NAS.md) before upgrading.

# 📜 License

Please see the accompanying LICENSE document contained within this repository.

# 🧑🏻 Social & Donate

- ❤️ it? Buy me a [☕](https://buymeacoffee.com/markrai)

![image](https://github.com/user-attachments/assets/e6327d1f-15db-467c-ad9d-ab6af0bc2666)
![image](https://github.com/user-attachments/assets/00195e6b-11f9-40cb-93c9-20ed2917a6b3)
