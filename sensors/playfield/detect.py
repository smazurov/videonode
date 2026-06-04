# /// script
# requires-python = ">=3.10"
# dependencies = ["numpy", "opencv-python-headless"]
# ///
"""Playfield detector — videonode-sensor subprocess backend.

Wire protocol (full-duplex over the inherited stdin/stdout socket):
  host -> child:  [u32 seq][u32 len][len bytes gray8 Y plane]   (native endian)
  child -> host:  "<seq> <kind> <conf> [x y w h]\\n"            (x,y,w,h normalized)

The Y plane is luma straight off the analysis composer's NV12 canvas. The
playfield is a near-rectangle viewed top-down whose physical-depth edges pop
under a brightness boost, so: CLAHE boost -> Canny -> close -> largest convex
4-gon -> bounding box, gated by a geometric prior (size / centering /
rectangularity) that yields the confidence the daemon thresholds on.
"""

import os
import struct
import sys

import cv2
import numpy as np

HDR = struct.Struct("=II")  # seq, len (native endian, matches the C++ host)

MIN_AREA_FRAC = 0.10
MAX_AREA_FRAC = 0.98
MAX_CENTER_OFFSET = 0.30  # playfield sits roughly centred


def read_exact(stream, n):
    buf = bytearray()
    while len(buf) < n:
        chunk = stream.read(n - len(buf))
        if not chunk:
            return None
        buf += chunk
    return bytes(buf)


def detect(gray):
    h, w = gray.shape
    frame_area = float(w * h)

    boosted = cv2.createCLAHE(clipLimit=3.0, tileGridSize=(8, 8)).apply(gray)
    blurred = cv2.GaussianBlur(boosted, (5, 5), 0)
    edges = cv2.Canny(blurred, 50, 150)
    edges = cv2.dilate(edges, np.ones((5, 5), np.uint8), iterations=1)

    contours, _ = cv2.findContours(edges, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
    if not contours:
        return None

    best = max(contours, key=cv2.contourArea)
    area = cv2.contourArea(best)
    area_frac = area / frame_area
    if area_frac < MIN_AREA_FRAC or area_frac > MAX_AREA_FRAC:
        return None

    peri = cv2.arcLength(best, True)
    approx = cv2.approxPolyDP(best, 0.02 * peri, True)
    rectangular = len(approx) == 4 and cv2.isContourConvex(approx)

    x, y, bw, bh = cv2.boundingRect(best)
    rect_area = float(bw * bh)
    solidity = area / rect_area if rect_area > 0 else 0.0

    cx = (x + bw / 2.0) / w
    cy = (y + bh / 2.0) / h
    center_off = max(abs(cx - 0.5), abs(cy - 0.5))
    if center_off > MAX_CENTER_OFFSET:
        return None

    conf = solidity
    conf *= 1.0 if rectangular else 0.6
    conf *= max(0.0, 1.0 - center_off / MAX_CENTER_OFFSET)
    conf = float(np.clip(conf, 0.0, 1.0))

    return (x / w, y / h, bw / w, bh / h, conf)


def main():
    width = int(os.environ.get("VN_WIDTH", "0"))
    height = int(os.environ.get("VN_HEIGHT", "0"))
    stdin = sys.stdin.buffer
    out = sys.stdout

    while True:
        hdr = read_exact(stdin, HDR.size)
        if hdr is None:
            break
        seq, length = HDR.unpack(hdr)
        payload = read_exact(stdin, length)
        if payload is None:
            break
        if width <= 0 or height <= 0 or length != width * height:
            out.write(f"{seq} none 0\n")
            out.flush()
            continue
        gray = np.frombuffer(payload, dtype=np.uint8).reshape(height, width)
        res = detect(gray)
        if res is None:
            out.write(f"{seq} none 0\n")
        else:
            x, y, bw, bh, conf = res
            out.write(f"{seq} bbox {conf:.4f} {x:.5f} {y:.5f} {bw:.5f} {bh:.5f}\n")
        out.flush()


if __name__ == "__main__":
    main()
