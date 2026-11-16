#!/usr/bin/env python3
"""Encode Natural Earth 110m coastlines + country borders as compact polylines
for web/index.html (the thin light lines drawn on the globe).

    curl -LO https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_110m_land.geojson
    curl -LO https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_110m_admin_0_boundary_lines_land.geojson
    python3 -I borders.py ne_110m_land.geojson ne_110m_admin_0_boundary_lines_land.geojson > borders.txt

Output: one line, base64 of a varint stream. For each polyline: point count,
then for every point the zigzag-encoded change in lon and lat (units of
0.1 degree) from the previous point (the first point of each line is relative
to the last point of the line before; the very first is relative to 0,0).
Coastlines come first, then borders; the stream starts with the number of
coastline polylines so the page can tell them apart. Each run length / delta
is an unsigned LEB128 varint.
"""
import base64
import json
import sys


def lines(path, closed):
    geo = json.load(open(path))
    for f in geo["features"]:
        g = f["geometry"]
        if g["type"] in ("Polygon", "MultiPolygon"):
            polys = g["coordinates"] if g["type"] == "MultiPolygon" else [g["coordinates"]]
            for poly in polys:
                for ring in poly:
                    yield ring
        elif g["type"] == "LineString":
            yield g["coordinates"]
        elif g["type"] == "MultiLineString":
            yield from g["coordinates"]


def quantise(line):
    out = []
    for lon, lat in line:
        p = (round(lon * 10), round(lat * 10))
        if not out or out[-1] != p:
            out.append(p)
    return out


def main():
    coast = [q for q in (quantise(l) for l in lines(sys.argv[1], True)) if len(q) > 1]
    border = [q for q in (quantise(l) for l in lines(sys.argv[2], False)) if len(q) > 1]
    out = bytearray()

    def varint(v):
        while True:
            b = v & 127
            v >>= 7
            out.append(b | (128 if v else 0))
            if not v:
                return

    def zig(v):
        return v * 2 if v >= 0 else -v * 2 - 1

    varint(len(coast))
    px, py = 0, 0
    for line in coast + border:
        varint(len(line))
        for x, y in line:
            varint(zig(x - px))
            varint(zig(y - py))
            px, py = x, y
    print(base64.b64encode(bytes(out)).decode())


if __name__ == "__main__":
    main()
