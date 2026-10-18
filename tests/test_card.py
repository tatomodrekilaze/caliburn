import unittest

from caliburn.places import site
from caliburn.raid import ridge_truth
from caliburn.sim import lost_track, nominal, on_final, one_key, replay_pairs, run
from caliburn.terrain import ground, sees
from caliburn.tracks import RADARS, anyone_sees


class GroundTests(unittest.TestCase):
    def test_the_yard_is_behind_the_ridge(self):
        poti = next(r for r in RADARS if r["name"] == "Poti")
        akh = site("Akhaltsikhe")
        kut = site("Kutaisi")
        sup = site("Supsa")
        self.assertLess(ground(kut["x"], kut["y"]), 400.0)
        self.assertGreater(ground(akh["x"], akh["y"]), 800.0)
        self.assertLess(ground(akh["x"], akh["y"]), 1600.0)
        self.assertTrue(sees(poti["x"], poti["y"], poti["z"], sup["x"], sup["y"], ground(sup["x"], sup["y"]) + 30.0))
        self.assertFalse(
            sees(poti["x"], poti["y"], poti["z"], akh["x"], akh["y"], ground(akh["x"], akh["y"]) + 20.0)
        )

    def test_ridge_path_is_dark_in_the_middle(self):
        truth = ridge_truth()
        dark = 0
        seen = 0
        for i in range(0, 180, 2):
            x, y, z = truth.pos(i)
            if anyone_sees(x, y, z):
                seen += 1
            else:
                dark += 1
        self.assertGreater(seen, 8)
        self.assertGreater(dark, 8)


class CardTests(unittest.TestCase):
    def test_nominal_pairs_the_sea_and_leaves_the_arrival(self):
        card = run(nominal())
        self.assertGreaterEqual(len(card["pairs"]), 2)
        self.assertEqual({row["truth"] for row in card["flown"]}, {"sea-a", "sea-b"})
        for row in card["flown"]:
            if row["home"] != "Poti":
                continue
            self.assertIsNotNone(row["min_range"])
            self.assertGreater(row["min_range"], card["hard_m"])
            self.assertLess(row["min_range"], 7000.0)
            self.assertIn(row["phase"], ("leave", "chute"))
        civil_lines = [line for line in card["log"] if "arrival" in line]
        self.assertEqual(civil_lines, [])

    def test_crossing_and_shadow_keep_one_identity(self):
        card = run(nominal())
        # Track nearest each sea truth just after the meet, and the ridge
        # truth before the mask and after it. One id each, no extra.
        def nearest(frame, name):
            tx = ty = None
            for row in frame["truths"]:
                if row[0] == name:
                    tx, ty = row[2], row[3]
            best = None
            best_d = 1.0e18
            for tr in frame["tracks"]:
                d = (tr[1] - tx) ** 2 + (tr[2] - ty) ** 2
                if d < best_d:
                    best_d = d
                    best = tr
            return best, best_d ** 0.5

        early = next(f for f in card["frames"] if f["t"] >= 20.0)
        late = next(f for f in card["frames"] if f["t"] >= 200.0)
        id_early, d_early = nearest(early, "ridge")
        id_late, d_late = nearest(late, "ridge")
        self.assertIsNotNone(id_early)
        self.assertIsNotNone(id_late)
        self.assertLess(d_early, 2500.0)
        self.assertLess(d_late, 4000.0)
        self.assertEqual(id_early[0], id_late[0])
        coasted = False
        for frame in card["frames"]:
            for tr in frame["tracks"]:
                if tr[0] == id_early[0] and tr[3] >= 4:
                    coasted = True
        self.assertTrue(coasted)
        # The two sea aircraft are still two tracks around the meet.
        meet = next(f for f in card["frames"] if f["t"] >= 90.0)
        ids = set()
        for name in ("sea-a", "sea-b"):
            tr, dist = nearest(meet, name)
            self.assertLess(dist, 2000.0, name)
            ids.add(tr[0])
        self.assertEqual(len(ids), 2)

    def test_one_key_stays_on_the_rail(self):
        card = run(one_key())
        self.assertEqual(card["pairs"], [])
        self.assertEqual(card["flown"], [])

    def test_final_is_withheld(self):
        card = run(on_final())
        self.assertEqual(card["pairs"], [])
        self.assertGreater(len(card["withheld"]), 0)
        self.assertEqual(card["flown"], [])

    def test_lost_track_recovers_outside_the_hard_deck(self):
        card = run(lost_track())
        self.assertEqual(len(card["flown"]), 1)
        row = card["flown"][0]
        self.assertIn(row["phase"], ("leave", "chute"))
        self.assertGreater(row["min_range"], card["hard_m"])
        self.assertTrue(any("leaving" in line for line in card["log"]))

    def test_replay_is_the_same_card(self):
        pairs_a, pairs_b, frames_a, frames_b = replay_pairs()
        self.assertEqual(pairs_a, pairs_b)
        self.assertEqual(frames_a[30]["tracks"], frames_b[30]["tracks"])
        self.assertEqual(frames_a[30]["craft"], frames_b[30]["craft"])


if __name__ == "__main__":
    unittest.main()
