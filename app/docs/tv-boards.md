# TV boards

Overview → TV boards (`/tv-boards`, permissions `tv:view`, `tv:edit`) keeps pages of incidents for
wall screens. Each board has an unguessable address `/tv/{slug}` that opens without signing in.

## What a board shows

- Active incidents, most severe first; within a severity, the newest first.
- Scope: business services, configuration items and teams. An incident shows when it matches any
  of them (its CI is chosen, its CI belongs to a chosen service, or it is routed to a chosen
  team). An empty scope shows every incident.
- Severity filter (none chosen = all), acknowledged incidents (on by default, dimmed), incidents
  in maintenance windows (off by default).
- Only the fields a screen needs: title, CI, services, team, severity, status, start time. No
  people, contacts or event payloads.

## Screen settings

Per board: refresh period (5 s – 10 min), language (Russian, English or the system default),
time zone of the clock (or the system default) and theme (dark by default). The screen keeps the
last data when the network drops, shows since when it has no connection, and asks again at once
when the network comes back.

## Who can open a board

Every board lists the addresses and networks it opens from (`192.168.5.40`, `10.20.0.0/16`, IPv6
too). The check runs on the server for every data request; a refused screen shows its own
address so it can be added.

If Umbrella stands behind a reverse proxy, add the proxy under **Trusted proxies** on the same
page. `X-Forwarded-For` is read only when the connection comes from a trusted proxy, from the
right, skipping trusted hops; otherwise the connection address is checked and the header is
ignored.

**New address** gives a board a new slug; screens on the old link stop getting data.
