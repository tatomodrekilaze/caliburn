"""The raid the card replays.

Positions are metres from the Poti origin in places.py. The fast aircraft
is the one that crosses Zekari. The two slower ones meet over the plain
and both keep going toward Supsa. The arrival into Kutaisi is civil.
"""

import math

from caliburn.air import COURSE, faf
from caliburn.places import site
from caliburn.terrain import ground

# Corridor used by the probe: (12000, -2000) toward (100000, -48000).
_CORRIDOR = (88000.0, -46000.0)
_CORRIDOR_LEN = math.hypot(*_CORRIDOR)
_CORRIDOR_HAT = (_CORRIDOR[0] / _CORRIDOR_LEN, _CORRIDOR[1] / _CORRIDOR_LEN)


def _meet():
    # Both pass through this point at t = 80 s, still aimed at Supsa.
    sup = site("Supsa")
    point = (10_000.0, -2_000.0)
    dx = sup["x"] - point[0]
    dy = sup["y"] - point[1]
    dist = math.hypot(dx, dy)
    unit = (dx / dist, dy / dist)
    perp = (unit[1], -unit[0])
    along = 46.0
    side = 16.0
    a = (unit[0] * along + perp[0] * side, unit[1] * along + perp[1] * side)
    b = (unit[0] * along - perp[0] * side, unit[1] * along - perp[1] * side)
    return point, a, b


class Truth:
    def __init__(self, name, kind, x, y, z, vx, vy, squawk=None, follow_ground=False, die=None):
        self.name = name
        self.kind = kind
        self.x0 = x
        self.y0 = y
        self.z0 = z
        self.vx = vx
        self.vy = vy
        self.squawk = squawk
        self.follow_ground = follow_ground
        self.die = die

    def alive(self, t):
        if self.die is None:
            return True
        return t < self.die

    def pos(self, t):
        x = self.x0 + self.vx * t
        y = self.y0 + self.vy * t
        if self.follow_ground:
            z = ground(x, y) + 160.0
        else:
            z = self.z0
        return x, y, z


def ridge_truth(speed=150.0):
    # 8 km before the mask the probe found near (38400, -15800).
    hat = _CORRIDOR_HAT
    x = 38400.0 - hat[0] * 8000.0
    y = -15800.0 - hat[1] * 8000.0
    return Truth(
        "ridge",
        "hostile",
        x,
        y,
        0.0,
        hat[0] * speed,
        hat[1] * speed,
        follow_ground=True,
    )


def sea_pair():
    point, va, vb = _meet()
    t_meet = 80.0
    a = Truth(
        "sea-a",
        "hostile",
        point[0] - va[0] * t_meet,
        point[1] - va[1] * t_meet,
        180.0,
        va[0],
        va[1],
    )
    b = Truth(
        "sea-b",
        "hostile",
        point[0] - vb[0] * t_meet,
        point[1] - vb[1] * t_meet,
        220.0,
        vb[0],
        vb[1],
    )
    return a, b


def arrival():
    x, y = faf()
    return Truth(
        "arrival",
        "civil",
        x,
        y,
        640.0,
        COURSE[0] * 72.0,
        COURSE[1] * 72.0,
        squawk="4261",
    )


def inside_final():
    x, y = faf()
    # A few kilometres inside the box, low, no squawk.
    x += COURSE[0] * 4000.0
    y += COURSE[1] * 4000.0
    return Truth(
        "in-final",
        "hostile",
        x,
        y,
        280.0,
        COURSE[0] * 42.0,
        COURSE[1] * 42.0,
    )


def one_sea(die=None):
    point, va, _vb = _meet()
    t_meet = 90.0
    return Truth(
        "sea-a",
        "hostile",
        point[0] - va[0] * t_meet,
        point[1] - va[1] * t_meet,
        200.0,
        va[0],
        va[1],
        die=die,
    )
