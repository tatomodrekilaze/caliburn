"""Kutaisi final, and the civil broadcast riding it.

The low layer of the Tbilisi FIR is Class G up to FL85 or 2,000 ft above
the ground, whichever is higher. The constraint on this card is the
instrument final into Kutaisi, which sits in that layer.
"""

import math

from caliburn.places import site

# Landing runway 25, true track rounded.
HEADING = math.radians(250.0)
COURSE = (math.sin(HEADING), math.cos(HEADING))
FINAL_M = 17_000.0
HALF_WIDTH = 1_400.0
ROOF_M = 1_500.0


def _apt():
    return site("Kutaisi")


def threshold():
    apt = _apt()
    return apt["x"], apt["y"]


def faf():
    x, y = threshold()
    return x - COURSE[0] * FINAL_M, y - COURSE[1] * FINAL_M


def on_final(x, y, z):
    if z > ROOF_M:
        return False
    x0, y0 = faf()
    x1, y1 = threshold()
    dx = x1 - x0
    dy = y1 - y0
    length = math.hypot(dx, dy)
    along = ((x - x0) * dx + (y - y0) * dy) / length
    cross = ((x - x0) * -dy + (y - y0) * dx) / length
    if along < -400.0 or along > length + 400.0:
        return False
    return abs(cross) < HALF_WIDTH


def path_touches_final(x0, y0, x1, y1, z):
    """A few samples is enough. The final is a long thin box."""
    for i in range(9):
        t = i / 8.0
        if on_final(x0 + (x1 - x0) * t, y0 + (y1 - y0) * t, z):
            return True
    return False
