# NAS Deployment Guide

This guide covers self-hosted Postbaby on a NAS or home server, including plain Docker Compose and Portainer.

> [!IMPORTANT]
> This is **not** a drop-in upgrade from the old static/Caddy image.
> The self-hosted container now runs the Go app on `8080`, requires persistent `/app/data`, and first-run account creation happens at `/setup`.
> Old browser-local `v1.35` data is **not** auto-migrated.
> If the app looks stale after upgrade, hard refresh once. Installed PWA users should close/reopen the app, and if needed clear site data.
> Read [README.md](../README.md) before upgrading.

## Choose a deployment path

Use the path that matches how your NAS is set up:

| Situation | Use |
|----------|-----|
| You can shell into the NAS and run Docker Compose directly | [`../docker-compose.yml`](../docker-compose.yml) |
| You use Portainer and it can build from a repo or uploaded compose file on the NAS | [`../docker-compose.yml`](../docker-compose.yml) |
| You use Portainer but do **not** want the NAS to build the image | Build/import `postbaby:local`, then use [`../docker-compose.portainer.yml`](../docker-compose.portainer.yml) |

## Common runtime facts

- Container port is always `8080`.
- Persistent app data must live at `/app/data` inside the container.
- The SQLite database file is `/app/data/postbaby.db`.
- First run on a new database redirects to `/setup`.
- Later visits use `/login`.
- No sync token is required. Self-hosted mode uses the built-in username/password setup flow and a browser session cookie.

## Environment variables

You can put these in a `.env` file next to the compose file, or set them in the Portainer stack UI.

| Variable | Required | Notes |
|----------|----------|-------|
| `POSTBABY_DATA_DIR` | No | Host path for persistent SQLite/auth data. Default is `./postbaby-data`, but on a NAS you should usually set an absolute path such as `/volume1/docker/postbaby/data`. |
| `POSTBABY_PORT` | No | Host port exposed to your network. Default is `8080` in `docker-compose.yml` and `9170` in `docker-compose.portainer.yml`. |
| `POSTBABY_COOKIE_SECURE` | No | Set `false` for direct HTTP testing. Set `true` when users reach the app over HTTPS through a reverse proxy. |
| `POSTBABY_SESSION_TTL` | No | Session lifetime. Default `720h`. |

Useful paths:

- Host default: `./postbaby-data`
- Host NAS example: `/volume1/docker/postbaby/data`
- Container data dir: `/app/data`
- Container DB file: `/app/data/postbaby.db`

## Option A: Docker Compose on the NAS

Use this when the NAS can build the image locally.

1. Clone or copy this repo onto the NAS.
2. Copy [`../.env.example`](../.env.example) to `.env`.
3. Set `POSTBABY_DATA_DIR` to a persistent NAS path.
4. Optionally change `POSTBABY_PORT` if you do not want to use `8080` externally.
5. Run:

```powershell
docker compose up --build -d
```

6. Open `http://<NAS-IP>:<POSTBABY_PORT>/`.

If you keep the default compose file unchanged, the host/container port mapping is `8080:8080`.

## Option B: Portainer stack that builds on the NAS

Use this when Portainer can work from a repo checkout or an uploaded compose file with build context available on the NAS.

1. Open **Stacks** in Portainer.
2. Add a new stack.
3. Use [`../docker-compose.yml`](../docker-compose.yml).
4. In **Environment variables**, set `POSTBABY_DATA_DIR` to a persistent NAS path.
5. Set `POSTBABY_PORT` if you want a different external port.
6. Deploy the stack.

If you want NAS users to hit `9170` instead of `8080`, set `POSTBABY_PORT=9170`.

## Option C: Portainer stack from an imported image tar

Use this when you want to build the image on another machine and only run it on the NAS.

### 1. Build the image tar

On a machine with Docker:

- Windows: run [`../generate_tar.bat`](../generate_tar.bat)
- Or manually run:

```powershell
docker build -t postbaby:local .
docker save -o postbaby.tar postbaby:local
```

Build the image for the same CPU architecture as the NAS that will run it.

### 2. Import the image into Portainer

1. In Portainer, open **Images**.
2. Choose **Import**.
3. Upload `postbaby.tar`.
4. Confirm the imported image tag is `postbaby:local`.

### 3. Deploy the Portainer stack

1. Open **Stacks** in Portainer.
2. Add a new stack.
3. Use [`../docker-compose.portainer.yml`](../docker-compose.portainer.yml).
4. In **Environment variables**, set `POSTBABY_DATA_DIR` to a persistent NAS path.
5. Set `POSTBABY_PORT` if needed. The default Portainer compose file exposes `9170:8080`.
6. Deploy the stack.

## After deploy

- Open `http://<NAS-IP>:<POSTBABY_PORT>/` or your reverse-proxy hostname.
- On a fresh database, Postbaby redirects to `/setup`.
- Create the first username/password account.
- On later visits, sign in through `/login`.
- Make sure the NAS firewall and router allow the external port you chose.

## Reverse proxy and HTTPS

- If users access Postbaby through HTTPS, set `POSTBABY_COOKIE_SECURE=true`.
- If users access it directly over plain HTTP on the LAN, leave `POSTBABY_COOKIE_SECURE=false`.

## Troubleshooting

- If the app looks broken after an upgrade, hard refresh once.
- Installed PWA users should close and reopen the app, and if needed clear site data.
- If you changed the host port, remember the container still listens on `8080` internally.
- If login/setup works but data disappears after restart, verify that `POSTBABY_DATA_DIR` points to persistent host storage and is mounted to `/app/data`.
