"""One raid, from plots to pairs to aircraft that leave.

Same seed, same pairs. The aircraft is finished when it is told to leave.
"""

import random

from caliburn.fly import HARD_M, step_craft
from caliburn.pair import SPEED, pair
from caliburn.places import site
from caliburn.raid import arrival, inside_final, one_sea, ridge_truth, sea_pair
from caliburn.tracks import scan as make_plots
from caliburn.tracks import Tracker

DT = 0.5


def _craft(cid, home, x, y):
    return {
        "id": cid,
        "home": home,
        "x": x,
        "y": y,
        "phase": "rail",
        "hdg": 0.0,
        "speed": SPEED,
        "min_range": 1.0e9,
        "leave_for": 0.0,
        "track": None,
        "truth": None,
        "blind": 0,
    }


def magazine():
    poti = site("Poti")
    akh = site("Akhaltsikhe")
    craft = []
    for i in range(3):
        craft.append(_craft("P%d" % (i + 1), "Poti", poti["x"] + 2000.0 + i * 160.0, poti["y"] + 500.0))
    craft.append(_craft("A1", "Akhaltsikhe", akh["x"] - 3500.0, akh["y"] + 2200.0))
    return craft


class Scenario:
    def __init__(self, name, truths, keys, t_end, seed):
        self.name = name
        self.truths = truths
        self.keys = keys
        self.t_end = t_end
        self.seed = seed


def nominal(seed=4):
    sea_a, sea_b = sea_pair()
    return Scenario("nominal", [sea_a, sea_b, ridge_truth(), arrival()], True, 260.0, seed)


def one_key():
    return Scenario("one-key", [sea_pair()[0], sea_pair()[1], arrival()], False, 36.0, 4)


def on_final():
    return Scenario("in-final", [inside_final(), arrival()], True, 36.0, 4)


def lost_track():
    return Scenario("lost", [one_sea(die=26.0)], True, 70.0, 4)


def _broadcasts(truths, t):
    rows = []
    for truth in truths:
        if truth.squawk is None or not truth.alive(t):
            continue
        x, y, z = truth.pos(t)
        rows.append({"x": x, "y": y, "z": z, "squawk": truth.squawk, "name": truth.name})
    return rows


def _launch(craft, track, truth_name):
    import math

    craft["phase"] = "fly"
    craft["track"] = track.id
    craft["truth"] = truth_name
    craft["hdg"] = math.atan2(track.y - craft["y"], track.x - craft["x"])
    craft["speed"] = SPEED
    craft["min_range"] = 1.0e9
    craft["blind"] = 0


def run(scenario):
    rng = random.Random(scenario.seed)
    tracker = Tracker()
    craft = magazine()
    truths = scenario.truths
    by_name = {tr.name: tr for tr in truths}
    log = []
    pairs = []
    withheld = set()
    frames = []
    steps = int(scenario.t_end / DT)
    tracks = []

    for k in range(steps):
        t = round(k * DT, 3)
        scanning = k % 4 == 0
        if scanning:
            plots = make_plots(truths, t, rng)
            tracks = tracker.step(plots, t)
            live = {tr.id: tr for tr in tracks}
            busy = {c["track"] for c in craft if c["track"]}
            pool = [tr for tr in tracks if tr.id not in busy]
            decision = pair(pool, craft, _broadcasts(truths, t), scenario.keys)
            truth_for = {}
            for tr in tracks:
                best = None
                best_d = 1.0e9
                for truth in truths:
                    x, y, _z = truth.pos(t)
                    d = (tr.x - x) ** 2 + (tr.y - y) ** 2
                    if d < best_d:
                        best_d = d
                        best = truth.name
                if best is not None and best_d < 3500.0 ** 2:
                    truth_for[tr.id] = best
            for tid in decision["withheld"]:
                withheld.add(tid)
            for cid, tid in decision["assign"]:
                if any(c["track"] == tid for c in craft):
                    continue
                chosen = next(c for c in craft if c["id"] == cid)
                guide = live[tid]
                _launch(chosen, guide, truth_for.get(tid))
                pairs.append((cid, tid))
                log.append("t %.0f  %s takes track %s" % (t, cid, tid))
            for c in craft:
                if c["phase"] != "fly" or c["track"] is None:
                    continue
                if c["track"] not in live:
                    c["blind"] += 1
                else:
                    c["blind"] = 0

        live = {tr.id: tr for tr in tracks}
        for c in craft:
            guide = None
            if c["phase"] == "fly" and c["blind"] < 2:
                guide = live.get(c["track"])
            was = c["phase"]
            step_craft(c, guide, DT)
            if was == "fly" and c["phase"] == "leave":
                log.append("t %.0f  %s leaving" % (t, c["id"]))
            if c["truth"] and c["phase"] != "rail":
                truth = by_name.get(c["truth"])
                if truth is not None:
                    x, y, _z = truth.pos(t)
                    d = ((c["x"] - x) ** 2 + (c["y"] - y) ** 2) ** 0.5
                    if d < c["min_range"]:
                        c["min_range"] = d

        if scanning:
            frames.append(
                {
                    "t": t,
                    "truths": [
                        [tr.name, tr.kind, round(tr.pos(t)[0]), round(tr.pos(t)[1])]
                        for tr in truths
                    ],
                    "tracks": [
                        [tr.id, round(tr.x), round(tr.y), tr.coast, round(tr.sigma())]
                        for tr in tracks
                    ],
                    "craft": [
                        [c["id"], c["phase"], round(c["x"]), round(c["y"])]
                        for c in craft
                    ],
                    "pairs": list(pairs),
                }
            )

    flown = []
    for c in craft:
        if c["phase"] == "rail":
            continue
        flown.append(
            {
                "id": c["id"],
                "home": c["home"],
                "phase": c["phase"],
                "truth": c["truth"],
                "track": c["track"],
                "min_range": None if c["min_range"] > 1.0e8 else round(c["min_range"], 1),
            }
        )

    return {
        "name": scenario.name,
        "keys": scenario.keys,
        "pairs": pairs,
        "withheld": sorted(withheld),
        "log": log,
        "frames": frames,
        "flown": flown,
        "hard_m": HARD_M,
    }


def replay_pairs(seed=4):
    a = run(nominal(seed))
    b = run(nominal(seed))
    return a["pairs"], b["pairs"], a["frames"], b["frames"]
