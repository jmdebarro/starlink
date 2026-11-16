#!/usr/bin/env python3
"""Rasterise Natural Earth 110m land to the equirectangular bitmap embedded in
web/index.html.

    curl -LO https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_110m_land.geojson
    python3 -I landmask.py ne_110m_land.geojson [W] > land.txt

Output: one line, base64 of a run-length stream. The image is W x W/2, rows from
north (lat +90) to south, columns from lon -180 eastwards. Runs alternate
ocean, land, ocean, ... starting with ocean, and continue across row ends;
each run length is an unsigned LEB128 varint. Pixel centres are tested with
the even-odd rule, so polygon holes (lakes) come out as water.
"""
import base64
import json
import sys


def rings(geo):
    for f in geo["features"]:
        g = f["geometry"]
        polys = g["coordinates"] if g["type"] == "MultiPolygon" else [g["coordinates"]]
        for poly in polys:
            for ring in poly:
                yield ring


def rasterise(geo, w):
    h = w // 2
    edges = []
    for ring in rings(geo):
        for (x0, y0), (x1, y1) in zip(ring, ring[1:] + ring[:1]):
            if y0 != y1:
                edges.append((x0, y0, x1, y1))
    img = bytearray(w * h)
    for r in range(h):
        lat = 90 - (r + 0.5) * 180 / h
        xs = []
        for x0, y0, x1, y1 in edges:
            if (y0 <= lat) != (y1 <= lat):
                xs.append(x0 + (lat - y0) * (x1 - x0) / (y1 - y0))
        xs.sort()
        for a, b in zip(xs[0::2], xs[1::2]):
            c0 = max(0, int((a + 180) * w / 360 - 0.5 + 0.999999))
            c1 = min(w - 1, int((b + 180) * w / 360 - 0.5))
            for c in range(c0, c1 + 1):
                img[r * w + c] = 1
    return img, h


def rle(img):
    out, cur, n = bytearray(), 0, 0
    def emit(v):
        while True:
            b = v & 127
            v >>= 7
            out.append(b | (128 if v else 0))
            if not v:
                return
    for p in img:
        if p == cur:
            n += 1
        else:
            emit(n)
            cur, n = p, 1
    emit(n)
    return bytes(out)


def main():
    geo = json.load(open(sys.argv[1]))
    w = int(sys.argv[2]) if len(sys.argv) > 2 else 720
    img, h = rasterise(geo, w)
    data = base64.b64encode(rle(img)).decode()
    print(f"{w}x{h} land={sum(img) / len(img):.3f} bytes={len(data)}", file=sys.stderr)
    print(data)


if __name__ == "__main__":
    main()
