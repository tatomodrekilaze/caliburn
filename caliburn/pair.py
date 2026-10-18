"""Pair aircraft to tracks.

The magazine is small, so the assignments are enumerated. A pair is legal
only when the standoff and the path out to it stay off the Kutaisi final.
The aim point is short of the track. Nothing on this card flies into it.
"""

import itertools
import math

from caliburn.air import on_final, path_touches_final
from caliburn.places import site

LEAVE_M = 2500.0
SPEED = 95.0
SIGMA_PAIR = 2200.0


def _standoff(pad, track):
    dx = track.x - pad[0]
    dy = track.y - pad[1]
    dist = math.hypot(dx, dy)
    if dist < LEAVE_M + 50.0:
        return pad[0], pad[1]
    scale = (dist - LEAVE_M) / dist
    return pad[0] + dx * scale, pad[1] + dy * scale


def _threatens(track, asset, horizon=420.0):
    dx = asset["x"] - track.x
    dy = asset["y"] - track.y
    dist = math.hypot(dx, dy)
    spd = track.speed()
    if spd < 15.0:
        return False
    closing = (dx * track.vx + dy * track.vy) / spd
    if closing < 20.0:
        return False
    time = dist / closing
    # Cross-track miss if it holds this velocity.
    along = closing * time
    miss = math.hypot(dx - track.vx / spd * along, dy - track.vy / spd * along)
    return time < horizon and miss < 6000.0


def label(track, broadcasts):
    for item in broadcasts:
        if math.hypot(track.x - item["x"], track.y - item["y"]) < 900.0:
            return "civil"
    if on_final(track.x, track.y, track.z):
        return "withheld"
    if track.coast > 0 or track.sigma() > SIGMA_PAIR:
        return "coasting"
    if track.hits < 6:
        return "unknown"
    if track.P[2][2] + track.P[3][3] > 400.0:
        return "unknown"
    return "hostile"


def assets():
    return [site("Supsa"), site("Akhaltsikhe"), site("Akhalkalaki")]


def pair(tracks, craft, broadcasts, keys_on):
    """craft entries need id, x, y, phase, home."""
    labels = {tr.id: label(tr, broadcasts) for tr in tracks}
    civil = [tid for tid, name in labels.items() if name == "civil"]
    withheld = [tid for tid, name in labels.items() if name == "withheld"]
    if not keys_on:
        return {"assign": [], "civil": civil, "withheld": withheld, "refused": "both keys"}

    hostile = [tr for tr in tracks if labels[tr.id] == "hostile"]
    free = [c for c in craft if c["phase"] == "rail"]
    if not hostile or not free:
        return {"assign": [], "civil": civil, "withheld": withheld, "refused": ""}

    # Precompute legal pairs and their time cost.
    legal = []
    for tr in hostile:
        if not any(_threatens(tr, asset) for asset in assets()):
            continue
        for c in free:
            sx, sy = _standoff((c["x"], c["y"]), tr)
            if path_touches_final(c["x"], c["y"], sx, sy, 400.0):
                continue
            if on_final(sx, sy, 400.0):
                continue
            gap = math.hypot(tr.x - c["x"], tr.y - c["y"])
            dist = math.hypot(sx - c["x"], sy - c["y"])
            opening = ((tr.x - c["x"]) * tr.vx + (tr.y - c["y"]) * tr.vy) / max(gap, 1.0)
            if opening > SPEED * 0.8:
                continue
            if dist / SPEED > 170.0:
                continue
            legal.append((dist / SPEED, c["id"], tr.id))

    by_track = {}
    for cost, cid, tid in legal:
        by_track.setdefault(tid, []).append((cost, cid))

    track_ids = sorted(by_track)
    craft_ids = sorted({cid for _, cid, _ in legal})
    if not track_ids:
        return {"assign": [], "civil": civil, "withheld": withheld, "refused": ""}

    best = None
    # Skip penalty keeps a threatening track from being left on the table
    # when some aircraft can still take it.
    skip = 500.0
    n = len(track_ids)
    for choice in itertools.product(*[list(range(len(craft_ids) + 1)) for _ in range(n)]):
        used = set()
        cost = 0.0
        mapping = []
        ok = True
        for i, pick in enumerate(choice):
            tid = track_ids[i]
            if pick == len(craft_ids):
                cost += skip
                continue
            cid = craft_ids[pick]
            if cid in used:
                ok = False
                break
            found = None
            for row in by_track[tid]:
                if row[1] == cid:
                    found = row[0]
                    break
            if found is None:
                ok = False
                break
            used.add(cid)
            cost += found
            mapping.append((cid, tid))
        if not ok:
            continue
        key = tuple(sorted(mapping))
        if best is None or (cost, key) < best[0]:
            best = ((cost, key), mapping)

    assign = best[1] if best else []
    return {"assign": assign, "civil": civil, "withheld": withheld, "refused": ""}
