"""Print whether the four engagements hold."""

from caliburn.sim import lost_track, nominal, on_final, one_key, replay_pairs, run


def _requirements(nom, key, final, lost, same_pairs):
    sea = {row["truth"] for row in nom["flown"]}
    poti_ok = all(
        row["min_range"] is not None and row["min_range"] > nom["hard_m"] and row["phase"] in ("leave", "chute")
        for row in nom["flown"]
    )
    rows = [
        ("TRK-01", "The two aircraft over the plain are still two tracks after they meet.", len(nom["pairs"]) >= 2 and sea == {"sea-a", "sea-b"}),
        ("TRK-02", "The ridge aircraft keeps one track through the shadow. The card's replay shows it coasting.", _coasted(nom)),
        ("AIR-01", "The Kutaisi arrival is never given an aircraft.", "arrival" not in sea),
        ("AIR-02", "A hostile sitting on the final is withheld. Nothing launches.", final["pairs"] == [] and len(final["withheld"]) > 0),
        ("KEY-01", "With one key off the magazine stays on the rail.", key["pairs"] == [] and key["flown"] == []),
        ("FLY-01", "Each sea aircraft is told to leave, and stays outside 1,500 m.", poti_ok and len(nom["flown"]) == 2),
        ("FLY-02", "When the track dies, the aircraft leaves and still stays outside 1,500 m.", len(lost["flown"]) == 1 and lost["flown"][0]["phase"] in ("leave", "chute") and lost["flown"][0]["min_range"] > lost["hard_m"]),
        ("REP-01", "The same seed writes the same pairs again.", same_pairs),
    ]
    return [{"id": ident, "text": text, "pass": bool(ok)} for ident, text, ok in rows]


def _coasted(card):
    # Same id near the ridge truth early and late, and that id coasts.
    def nearest(frame):
        tx = ty = None
        for row in frame["truths"]:
            if row[0] == "ridge":
                tx, ty = row[2], row[3]
        if tx is None:
            return None
        best = None
        best_d = 1.0e18
        for tr in frame["tracks"]:
            d = (tr[1] - tx) ** 2 + (tr[2] - ty) ** 2
            if d < best_d:
                best_d = d
                best = tr
        if best is None or best_d > 4000.0 ** 2:
            return None
        return best

    early = next((f for f in card["frames"] if f["t"] >= 20.0), None)
    late = next((f for f in card["frames"] if f["t"] >= 200.0), None)
    if early is None or late is None:
        return False
    a = nearest(early)
    b = nearest(late)
    if a is None or b is None or a[0] != b[0]:
        return False
    return any(tr[0] == a[0] and tr[3] >= 4 for frame in card["frames"] for tr in frame["tracks"])


def main():
    nom = run(nominal())
    key = run(one_key())
    final = run(on_final())
    lost = run(lost_track())
    pairs_a, pairs_b, _fa, _fb = replay_pairs()
    reqs = _requirements(nom, key, final, lost, pairs_a == pairs_b)
    passed = sum(1 for item in reqs if item["pass"])
    print("Caliburn  %d/%d" % (passed, len(reqs)))
    for item in reqs:
        mark = "pass" if item["pass"] else "fail"
        print("  %s  %s  %s" % (mark, item["id"], item["text"]))


if __name__ == "__main__":
    main()
