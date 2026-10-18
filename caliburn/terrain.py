"""Ground height and line of sight.

The ridge stands in for Zekari, between the Zestafoni side and Akhaltsikhe.
Guria is the hill country south of the plain. Heights are a sketch of the
real relief, not a survey.
"""

import math

from caliburn.places import site

# Crest line, north end to south end, metres in the local frame.
# Crest from the Likhi side, over Zekari, down toward Samtskhe.
# It crosses the Poti to Akhaltsikhe line, which is why the coast radar
# does not see the converter yard.
# South of Kutaisi, through the Zekari heights, stopping short of the town
# so the converter yard sits in the shadow instead of on the crest.
CREST = (
    (46_000.0, -16_000.0),
    (70_000.0, -28_000.0),
    (95_200.0, -30_600.0),
    (102_000.0, -40_000.0),
)
RIDGE_PEAK = 2_350.0
RIDGE_SIGMA = 3_400.0

GURIA = (26_000.0, -24_000.0)
GURIA_PEAK = 780.0
GURIA_SIGMA = 10_500.0

EARTH_R = 6_371_000.0
CLEARANCE = 18.0


def _along_cross(x, y, a, b):
    ax, ay = a
    bx, by = b
    dx = bx - ax
    dy = by - ay
    length = math.hypot(dx, dy)
    along = ((x - ax) * dx + (y - ay) * dy) / length
    cross = ((x - ax) * -dy + (y - ay) * dx) / length
    return along, cross, length


def ground(x, y):
    h = 0.0
    if x > 2_000.0:
        h += min(380.0, (x - 2_000.0) * 0.0028)

    dx = x - GURIA[0]
    dy = y - GURIA[1]
    h += GURIA_PEAK * math.exp(-(dx * dx + dy * dy) / (2.0 * GURIA_SIGMA * GURIA_SIGMA))

    ridge = 0.0
    for i in range(len(CREST) - 1):
        along, cross, length = _along_cross(x, y, CREST[i], CREST[i + 1])
        if along < -5_000.0 or along > length + 5_000.0:
            continue
        edge = 1.0
        if along < 0.0:
            edge = math.exp(-(along * along) / (2.0 * 4_000.0 * 4_000.0))
        elif along > length:
            over = along - length
            edge = math.exp(-(over * over) / (2.0 * 4_000.0 * 4_000.0))
        piece = RIDGE_PEAK * edge * math.exp(-(cross * cross) / (2.0 * RIDGE_SIGMA * RIDGE_SIGMA))
        if piece > ridge:
            ridge = piece
    h += ridge

    # Samtskhe is high country, but the town is not the crest.
    akh = site("Akhaltsikhe")
    dakh = math.hypot(x - akh["x"], y - akh["y"])
    h += 620.0 * math.exp(-(dakh * dakh) / (2.0 * 18_000.0 * 18_000.0))

    jav = site("Akhalkalaki")
    djav = math.hypot(x - jav["x"], y - jav["y"])
    h += 900.0 * math.exp(-(djav * djav) / (2.0 * 26_000.0 * 26_000.0))
    return h


def sees(ax, ay, az, bx, by, bz, step=450.0):
    dist = math.hypot(bx - ax, by - ay)
    if dist < 80.0:
        return True
    n = max(2, int(dist / step))
    for i in range(1, n):
        t = i / n
        x = ax + (bx - ax) * t
        y = ay + (by - ay) * t
        z_ray = az + (bz - az) * t
        d1 = t * dist
        d2 = (1.0 - t) * dist
        bulge = d1 * d2 / (2.0 * EARTH_R)
        if ground(x, y) + CLEARANCE > z_ray - bulge:
            return False
    return True
