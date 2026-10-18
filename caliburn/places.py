"""Places on the east-west spine.

Lat and lon are the public ones, rounded. A few hundred metres does not
matter for line of sight across this map. Origin is near Poti.
"""

import math

LAT0 = 42.10
LON0 = 41.70
M_LAT = 111_320.0
M_LON = 111_320.0 * math.cos(math.radians(LAT0))


def xy(lat, lon):
    return ((lon - LON0) * M_LON, (lat - LAT0) * M_LAT)


# name, lat, lon, role
_RAW = [
    ("Poti", 42.146, 41.672, "sea"),
    ("Supsa", 42.016, 41.758, "terminal"),
    ("Kutaisi", 42.1767, 42.4826, "airport"),
    ("Batumi", 41.6103, 41.5996, "airport"),
    ("Zestafoni", 42.109, 43.038, "substation"),
    ("Akhaltsikhe", 41.639, 42.986, "converters"),
    ("Akhalkalaki", 41.405, 43.486, "gauge"),
    ("Zekari", 41.825, 42.852, "pass"),
]


def _build():
    out = {}
    for name, lat, lon, role in _RAW:
        x, y = xy(lat, lon)
        out[name] = {"lat": lat, "lon": lon, "x": x, "y": y, "role": role}
    return out


SITES = _build()


def site(name):
    return SITES[name]
