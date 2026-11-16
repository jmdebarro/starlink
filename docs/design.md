# Starlink tracker — design

A Go service downloads Starlink TLEs from CelesTrak every 2 hours, computes
live satellite positions with SGP4, and serves them as JSON. A single
self-contained web page draws them on a pixelated, spinnable globe. The page
will later be hosted on the blog (debarro.dev/starlink/, like /collage/); the
Go service runs in a k8s pod on a home laptop (owner deploys it themselves).

## Layout

```
starlink/
├── go.mod
├── cmd/
│   ├── starlink/main.go   # CLI: fetch (or read cache) and print a summary
│   └── server/main.go     # HTTP service: refresh loop + API + serves web/
├── internal/
│   ├── tle/               # download + cache (tle.go), parse, fake data generator
│   └── orbit/             # SGP4 propagation → lat/lon/alt
├── web/
│   └── index.html         # the globe page, self-contained, no build step
└── data/                  # cached TLEs (git-ignored)
```

## Go service (`cmd/server`)

Configuration: every flag also reads an environment variable (12-factor, so
the k8s Deployment can set env vars). Flags win over env.

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `-addr` | `ADDR` | `:8080` | listen address |
| `-data` | `DATA_FILE` | `data/starlink.tle` | TLE cache file |
| `-fake` | `FAKE=1` | off | use generated fake TLEs, never touch CelesTrak or the cache file |
| `-web` | `WEB_DIR` | `web` | directory served at `/`; empty string disables |
| `-cors-origin` | `CORS_ORIGIN` | `*` | value of `Access-Control-Allow-Origin` on `/api/*` |
| `-refresh` | `REFRESH` | `2h` | how often to reload TLEs |

Behaviour:
- At startup load TLEs (`tle.Load`, which already handles cache/2h/stale
  fallback). If that fails and there is no cache, keep running: `/healthz`
  returns 503 and the loader retries every 10 minutes until it succeeds.
- A background goroutine reloads every `-refresh`. The parsed satellite set is
  swapped in atomically (readers never see a half-built set).
- Positions are computed for 60-second windows: `t0` = now rounded down to a
  multiple of 60 s, `t1 = t0 + 60`. The computed snapshot is cached until the
  window changes, so many requests per minute cost one computation.
- Satellites whose propagation fails (decayed, bad TLE) are skipped, never crash
  the server. Malformed TLE entries are skipped at parse time.
- Logs go to stdout (k8s collects them): startup config, each reload (source,
  satellite count), errors.
- Graceful shutdown on SIGTERM/SIGINT (k8s sends SIGTERM).

### API

All JSON responses are gzip-compressed when the client sends
`Accept-Encoding: gzip`, and carry the CORS header.

`GET /api/positions`
```json
{
  "source": "fake" | "download" | "cache" | "cache (stale)",
  "tleUpdated": "2026-11-15T23:36:31Z",
  "t0": 1763249760,
  "step": 60,
  "count": 9876,
  "sats": [[44714, 51.23, -12.40, 551, 50.10, -9.85, 551], ...]
}
```
Each `sats` row: `[noradId, lat0, lon0, alt0, lat1, lon1, alt1]` — degrees
(2 decimals) and km above the WGS84 ellipsoid (integer), at `t0` and `t1`.
`Cache-Control: max-age=` seconds until `t1`.

`GET /api/catalog` → `{"sats": [[44714, "STARLINK-1008", "2019-074A", 53.05, 95.6], ...]}`
— `[noradId, name, launchId, inclination°, period min]`, read from the TLE
lines (`tle.Record.Info`). Used for hover labels and the satellite panel;
changes only on reload.

`GET /healthz` → `200 ok` once TLEs are loaded, else `503`.

### Fake data (`internal/tle`, `-fake`)

Generates syntactically valid TLEs (correct column layout and mod-10
checksums) laid out like real Starlink shells, epoch = now:

| Shell | Inclination | Altitude | Planes × sats |
|---|---|---|---|
| 1 | 53.0° | 550 km | 72 × 22 |
| 2 | 53.2° | 540 km | 72 × 22 |
| 3 | 70.0° | 570 km | 36 × 20 |
| 4 | 97.6° | 560 km | 6 × 58 |
| 5 | 43.0° | 530 km | 28 × 54 |

Planes evenly spaced in RAAN, satellites evenly spaced in mean anomaly with a
small per-plane phase offset; eccentricity ~0.0001, B* 0, mean motion from the
altitude. Names `FAKE-0001`…, NORAD ids from 90000. Fake data is never written
to the cache file, so it can't block the first real download.

## Web page (`web/index.html`)

- One self-contained HTML file (inline CSS/JS, no build, no frameworks). Only
  external resource allowed: the JetBrains Mono Google Font.
- API base URL: `?api=https://host` query parameter, else same origin. (The
  blog copy will point at the k8s service.)
- Look: matches the blog's "Nocturne" palette — warm black `#0B0A09`, bone
  `#E8E0D0`, muted `#A39886`, vermilion accent `#E8694A` — and the Glyphshift
  page: full-bleed canvas, thin HUD strips top and bottom in JetBrains Mono,
  11px uppercase, letter-spaced.
- Everything pixelated: render the scene to a small canvas (globe ~200 px
  across) and scale it up by an integer factor with
  `image-rendering: pixelated`. Limited palette; shading quantised with
  ordered (Bayer) dithering. No anti-aliasing anywhere.
- Earth: orthographic projection, land/ocean from a low-res land bitmap
  embedded in the file, rasterised from Natural Earth 110m land (public
  domain). Day/night from the real sun position, quantised + dithered; the
  night side stays dimly visible.
- Earth colours (the one exception to the Nocturne palette): blue ocean, green
  land, tan deserts and white ice (rough lat/lon boxes, 5 shades each) and a
  thin blue atmosphere ring.
- Coastlines and country borders: Natural Earth 110m polylines embedded in the
  file (`web/tools/borders.py`), drawn as 1-pixel Bresenham lines on the front
  hemisphere, pale blue (coast) and bone (borders), dimmer at night.
- Satellites: single vermilion pixels at their real altitude (slightly off the
  surface); hidden when behind the globe. Positions interpolated between `t0`
  and `t1` in 3-D (not lat/lon, so the ±180° wrap is safe); refetch when a new
  window starts.
- Interaction: drag to spin (mouse + touch, with inertia), wheel/pinch zoom
  within limits, slow auto-spin that resumes after a few seconds idle,
  double-click/tap resets. Hover shows the nearest satellite's name and
  altitude.
- Click/tap a satellite: a panel opens (right side on desktop, bottom sheet on
  phones) with an animated pixel-art satellite (one wing for V1.0/V1.5, two for
  V2 Mini — generation estimated from the launch year; drawn as a tiny 3-D model
  (box chassis, wing panels, depth buffer) turned to a per-satellite angle; wing size, panel grid,
  tint, light colour and backdrop vary per satellite, seeded by NORAD id, with a
  dither dissolve + orange border flash on switch), NORAD id, launch id,
  live altitude / speed / lat / lon / sunlit-or-shadow, inclination, period,
  and an N2YO tracking link (hidden for fake data). × or Esc closes it. `prefers-reduced-motion`: no auto-spin.
- HUD: brand, satellite count, data source (a clear `FAKE DATA` tag when
  fake), TLE updated time, UTC clock; `API OFFLINE — RETRYING` state.
- `?embed` hides the HUD (same convention as `/collage/?embed`).
- Works at phone width; 60 fps on a phone.
