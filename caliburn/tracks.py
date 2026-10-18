"""Radars and the tracker.

Poti watches the sea. Akhaltsikhe watches the yard. Neither sees through
Zekari. A miss in that shadow is a coast. Two plots that can swap are
kept as two hypotheses until one of them pulls ahead.
"""

import copy
import math

from caliburn.places import site
from caliburn.terrain import ground, sees

SCAN_S = 2.0
SIGMA_R = 42.0
SIGMA_AZ = math.radians(0.45)
GATE = 13.5
CONFIRM_HITS = 3
VISIBLE_MISSES = 4
COAST_LIMIT_S = 210.0
ACCEL = 1.6


def radars():
    poti = site("Poti")
    akh = site("Akhaltsikhe")
    return (
        {
            "name": "Poti",
            "x": poti["x"] + 1800.0,
            "y": poti["y"] - 600.0,
            "z": ground(poti["x"], poti["y"]) + 36.0,
            "range": 95000.0,
        },
        {
            "name": "Akhaltsikhe",
            "x": akh["x"] - 2400.0,
            "y": akh["y"] + 1600.0,
            "z": ground(akh["x"], akh["y"]) + 28.0,
            "range": 80000.0,
        },
    )


RADARS = radars()


def visible_from(radar, x, y, z):
    if z < ground(x, y) + 40.0:
        return False
    dist = math.hypot(x - radar["x"], y - radar["y"])
    if dist > radar["range"] or dist < 400.0:
        return False
    return sees(radar["x"], radar["y"], radar["z"], x, y, z)


def anyone_sees(x, y, z):
    return any(visible_from(radar, x, y, z) for radar in RADARS)


def scan(truths, t, rng):
    plots = []
    for radar in RADARS:
        for truth in truths:
            if not truth.alive(t):
                continue
            x, y, z = truth.pos(t)
            if not visible_from(radar, x, y, z):
                continue
            if rng.random() > 0.93:
                continue
            dx = x - radar["x"]
            dy = y - radar["y"]
            r = math.hypot(dx, dy)
            az = math.atan2(dx, dy)
            r = max(200.0, r + rng.gauss(0.0, SIGMA_R))
            az = az + rng.gauss(0.0, SIGMA_AZ)
            plots.append(
                {
                    "radar": radar["name"],
                    "x": radar["x"] + r * math.sin(az),
                    "y": radar["y"] + r * math.cos(az),
                    "z": z + rng.gauss(0.0, 25.0),
                    "r": r,
                    "az": az,
                    "t": t,
                }
            )
        if radar["name"] != "Poti":
            continue
        for _ in range(5):
            rr = rng.uniform(2000.0, 28000.0)
            az = rng.uniform(-2.4, 1.2)
            plots.append(
                {
                    "radar": "Poti",
                    "x": radar["x"] + rr * math.sin(az),
                    "y": radar["y"] + rr * math.cos(az),
                    "z": rng.uniform(40.0, 300.0),
                    "r": rr,
                    "az": az,
                    "t": t,
                }
            )
    plots.sort(key=lambda p: (p["radar"], round(p["x"]), round(p["y"])))
    return plots


def _zeros(n):
    return [[0.0] * n for _ in range(n)]


def _mul(a, b):
    n = len(a)
    m = len(b[0])
    p = len(b)
    out = [[0.0] * m for _ in range(n)]
    for i in range(n):
        for k in range(p):
            aik = a[i][k]
            if aik == 0.0:
                continue
            row = b[k]
            dest = out[i]
            for j in range(m):
                dest[j] += aik * row[j]
    return out


def _transpose(a):
    return [list(col) for col in zip(*a)]


def _meas_r(plot):
    r = max(plot["r"], 200.0)
    ux, uy = math.sin(plot["az"]), math.cos(plot["az"])
    px, py = math.cos(plot["az"]), -math.sin(plot["az"])
    along = SIGMA_R * SIGMA_R
    cross = (r * SIGMA_AZ) ** 2
    return [
        [along * ux * ux + cross * px * px, along * ux * uy + cross * px * py],
        [along * uy * ux + cross * py * px, along * uy * uy + cross * py * py],
    ]


def _inv2(m):
    det = m[0][0] * m[1][1] - m[0][1] * m[1][0]
    if abs(det) < 1e-9:
        det = 1e-9 if det >= 0.0 else -1e-9
    return [[m[1][1] / det, -m[0][1] / det], [-m[1][0] / det, m[0][0] / det]]


class Track:
    def __init__(self, tid, x, y, z, vx, vy, t):
        self.id = tid
        self.t = t
        self.x = x
        self.y = y
        self.z = z
        self.vx = vx
        self.vy = vy
        self.hits = CONFIRM_HITS
        self.miss_vis = 0
        self.coast = 0
        self.dropped = False
        self.P = _zeros(4)
        self.P[0][0] = 80.0 ** 2
        self.P[1][1] = 80.0 ** 2
        self.P[2][2] = 8.0 ** 2
        self.P[3][3] = 8.0 ** 2

    def speed(self):
        return math.hypot(self.vx, self.vy)

    def sigma(self):
        return math.sqrt(max(1.0, self.P[0][0] + self.P[1][1]))


class Tentative:
    def __init__(self, plot):
        self.x0 = plot["x"]
        self.y0 = plot["y"]
        self.t0 = plot["t"]
        self.x = plot["x"]
        self.y = plot["y"]
        self.z = plot["z"]
        self.t = plot["t"]
        self.hits = 1


def _predict(tr, dt):
    if dt <= 0.0:
        return
    tr.x += tr.vx * dt
    tr.y += tr.vy * dt
    tr.t += dt
    f = _zeros(4)
    for i in range(4):
        f[i][i] = 1.0
    f[0][2] = dt
    f[1][3] = dt
    p = _mul(_mul(f, tr.P), _transpose(f))
    q = ACCEL * ACCEL
    dt2 = dt * dt
    dt3 = dt2 * dt
    dt4 = dt2 * dt2
    q00 = q * dt4 / 4.0
    q02 = q * dt3 / 2.0
    q22 = q * dt2
    p[0][0] += q00
    p[0][2] += q02
    p[2][0] += q02
    p[2][2] += q22
    p[1][1] += q00
    p[1][3] += q02
    p[3][1] += q02
    p[3][3] += q22
    tr.P = p


def _mahal(tr, plot):
    mx = plot["x"] - tr.x
    my = plot["y"] - tr.y
    r = _meas_r(plot)
    s = [
        [tr.P[0][0] + r[0][0], tr.P[0][1] + r[0][1]],
        [tr.P[1][0] + r[1][0], tr.P[1][1] + r[1][1]],
    ]
    inv = _inv2(s)
    return mx * (inv[0][0] * mx + inv[0][1] * my) + my * (inv[1][0] * mx + inv[1][1] * my)


def _update(tr, plot):
    mx = plot["x"] - tr.x
    my = plot["y"] - tr.y
    r = _meas_r(plot)
    s = [
        [tr.P[0][0] + r[0][0], tr.P[0][1] + r[0][1]],
        [tr.P[1][0] + r[1][0], tr.P[1][1] + r[1][1]],
    ]
    inv = _inv2(s)
    k = [[0.0, 0.0] for _ in range(4)]
    for i in range(4):
        k[i][0] = tr.P[i][0] * inv[0][0] + tr.P[i][1] * inv[1][0]
        k[i][1] = tr.P[i][0] * inv[0][1] + tr.P[i][1] * inv[1][1]
    tr.x += k[0][0] * mx + k[0][1] * my
    tr.y += k[1][0] * mx + k[1][1] * my
    tr.vx += k[2][0] * mx + k[2][1] * my
    tr.vy += k[3][0] * mx + k[3][1] * my
    eye = _zeros(4)
    for i in range(4):
        eye[i][i] = 1.0
        eye[i][0] -= k[i][0]
        eye[i][1] -= k[i][1]
    tr.P = _mul(eye, tr.P)
    for i in range(4):
        if tr.P[i][i] < 1.0:
            tr.P[i][i] = 1.0
    tr.z = 0.65 * tr.z + 0.35 * plot["z"]
    tr.hits += 1
    tr.miss_vis = 0
    tr.coast = 0


def _masked(tr):
    # The estimate lags the aircraft. If the next few seconds of the
    # velocity run into the ridge, a miss is the shadow starting, not a drop.
    points = [(tr.x, tr.y, tr.z)]
    for ahead in (3.0, 7.0, 12.0):
        x = tr.x + tr.vx * ahead
        y = tr.y + tr.vy * ahead
        z = tr.z
        g = ground(x, y)
        if z < g + 80.0:
            z = g + 160.0
        points.append((x, y, z))
    return any(not anyone_sees(x, y, z) for x, y, z in points)


def _miss_cost(tr):
    if _masked(tr):
        return 0.35
    return 8.0


def _assign(tracks, plots):
    live = [tr for tr in tracks if not tr.dropped]
    live.sort(key=lambda tr: tr.id)
    if not live:
        return [], 0.0, None

    options = []
    for tr in live:
        opts = []
        for pi, plot in enumerate(plots):
            d2 = _mahal(tr, plot)
            if d2 < GATE:
                opts.append((d2, pi))
        opts.sort()
        opts = opts[:3]
        opts.append((None, None))
        options.append((tr, opts))

    best = None
    second = None

    def consider(cost, mapping):
        nonlocal best, second
        key = tuple(-1 if pi is None else pi for _, pi in mapping)
        row = (cost, key, mapping)
        if best is None or (cost, key) < (best[0], best[1]):
            second = best
            best = row
        elif second is None or (cost, key) < (second[0], second[1]):
            second = row

    def walk(i, used, cost, mapping):
        if i == len(options):
            consider(cost, mapping)
            return
        tr, opts = options[i]
        for d2, pi in opts:
            if pi is not None and pi in used:
                continue
            extra = _miss_cost(tr) if pi is None else d2
            nxt = used if pi is None else used | {pi}
            walk(i + 1, nxt, cost + extra, mapping + [(tr.id, pi)])

    walk(0, set(), 0.0, [])
    alt = None
    if second is not None and second[0] - best[0] < 2.0 and second[1] != best[1]:
        alt = (second[2], second[0])
    return best[2], best[0], alt


def _apply(hyp, mapping, plots, t):
    by_id = {tr.id: tr for tr in hyp.tracks}
    used = set()
    for tid, pi in mapping:
        tr = by_id.get(tid)
        if tr is None or tr.dropped:
            continue
        if pi is None:
            if _masked(tr):
                tr.coast += 1
                tr.miss_vis = 0
                if tr.coast * SCAN_S > COAST_LIMIT_S:
                    tr.dropped = True
            else:
                tr.coast = 0
                tr.miss_vis += 1
                if tr.miss_vis >= VISIBLE_MISSES:
                    tr.dropped = True
            continue
        _update(tr, plots[pi])
        used.add(pi)

    for pi, plot in enumerate(plots):
        if pi in used:
            continue
        for tr in hyp.tracks:
            if tr.dropped:
                continue
            if _mahal(tr, plot) < GATE:
                _update(tr, plot)
                used.add(pi)
                break

    still = []
    for tent in hyp.tents:
        if t - tent.t > 8.0:
            continue
        grabbed = None
        best_d = 1e9
        for pi, plot in enumerate(plots):
            if pi in used:
                continue
            d = math.hypot(plot["x"] - tent.x, plot["y"] - tent.y)
            if d < best_d and d < 800.0:
                best_d = d
                grabbed = pi
        if grabbed is None:
            still.append(tent)
            continue
        plot = plots[grabbed]
        used.add(grabbed)
        tent.x = plot["x"]
        tent.y = plot["y"]
        tent.z = plot["z"]
        tent.t = t
        tent.hits += 1
        if tent.hits >= CONFIRM_HITS:
            dt = max(0.5, t - tent.t0)
            vx = (tent.x - tent.x0) / dt
            vy = (tent.y - tent.y0) / dt
            spd = math.hypot(vx, vy)
            if 18.0 <= spd <= 230.0:
                hyp.tracks.append(Track(hyp.next_id, tent.x, tent.y, tent.z, vx, vy, t))
                hyp.next_id += 1
        else:
            still.append(tent)
    hyp.tents = still

    for pi, plot in enumerate(plots):
        if pi in used:
            continue
        near = False
        for tr in hyp.tracks:
            if not tr.dropped and math.hypot(plot["x"] - tr.x, plot["y"] - tr.y) < 500.0:
                near = True
                break
        if not near:
            hyp.tents.append(Tentative(plot))

    hyp.tracks = [tr for tr in hyp.tracks if not tr.dropped]
    hyp.t = t


class Hyp:
    def __init__(self):
        self.tracks = []
        self.tents = []
        self.score = 0.0
        self.t = 0.0
        self.next_id = 1


class Tracker:
    def __init__(self):
        self.hyps = [Hyp()]
        self.t = 0.0

    def step(self, plots, t):
        dt = t - self.t
        nxt = []
        for hyp in self.hyps:
            for tr in hyp.tracks:
                _predict(tr, dt)
            mapping, cost, alt = _assign(hyp.tracks, plots)
            keep = copy.deepcopy(hyp)
            _apply(keep, mapping, plots, t)
            keep.score -= cost
            nxt.append(keep)
            if alt is not None:
                other = copy.deepcopy(hyp)
                _apply(other, alt[0], plots, t)
                other.score -= alt[1]
                nxt.append(other)
        nxt.sort(key=lambda h: (-h.score, h.next_id))
        self.hyps = nxt[:6]
        self.t = t
        return self.live()

    def live(self):
        best = self.hyps[0]
        rows = [tr for tr in best.tracks if not tr.dropped]
        rows.sort(key=lambda tr: tr.id)
        return rows
