"""Point-mass aircraft.

It flies at a point short of the track. Crossing that distance, or losing
the track, is the order to leave. The parachute is the end of the flight.
There is no impact mode.
"""

import math

from caliburn.pair import LEAVE_M, SPEED

HARD_M = 1500.0
TURN = 0.55


def step_craft(craft, track, dt):
    if craft["phase"] == "rail":
        return
    if craft["phase"] == "chute":
        craft["speed"] += (12.0 - craft["speed"]) * min(1.0, dt / 4.0)
        craft["x"] += math.cos(craft["hdg"]) * craft["speed"] * dt
        craft["y"] += math.sin(craft["hdg"]) * craft["speed"] * dt
        craft["leave_for"] += dt
        return

    lost = track is None
    if track is not None:
        dist = math.hypot(track.x - craft["x"], track.y - craft["y"])
        craft["min_range"] = min(craft["min_range"], dist)
        if dist < LEAVE_M:
            lost = True
        else:
            dx = track.x - craft["x"]
            dy = track.y - craft["y"]
            # Standoff sits LEAVE_M short of the track, on the line between them.
            scale = (dist - LEAVE_M) / dist
            tx = craft["x"] + dx * scale
            ty = craft["y"] + dy * scale
            want = math.atan2(ty - craft["y"], tx - craft["x"])
            err = (want - craft["hdg"] + math.pi) % (2 * math.pi) - math.pi
            craft["hdg"] += max(-TURN * dt, min(TURN * dt, err))

    if craft["phase"] == "fly" and lost:
        craft["phase"] = "leave"
        craft["hdg"] = craft["hdg"] + 2.4
        craft["leave_for"] = 0.0

    craft["x"] += math.cos(craft["hdg"]) * craft["speed"] * dt
    craft["y"] += math.sin(craft["hdg"]) * craft["speed"] * dt

    if craft["phase"] == "leave":
        craft["leave_for"] += dt
        if craft["leave_for"] > 10.0:
            craft["phase"] = "chute"
