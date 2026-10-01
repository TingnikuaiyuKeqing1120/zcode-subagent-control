#!/usr/bin/env python3
"""纯标准库生成两个 ZCode 小工具的区分性图标（PNG + ICO）。

设计语言：同族不同形——统一的深色圆角方块（ZCode 深色底），
  zcode-subagent-control = 三色滑杆（权限档位：红=完全访问/琥珀=变更前确认/绿=自动编辑）
  zcode-dashboard-go     = 青色上升柱组（统计面板）
渲染：解析式 SDF + 4x 超采样，输出 RGBA PNG 与多尺寸 ICO。
"""
import math
import os
import struct
import sys
import zlib

SS = 4  # 超采样倍数


def hex2rgb(h):
    h = h.lstrip("#")
    return tuple(int(h[i : i + 2], 16) for i in (0, 2, 4))


# ---------- SDF 数学 ----------

def dist_seg(px, py, ax, ay, bx, by):
    dx, dy = bx - ax, by - ay
    l2 = dx * dx + dy * dy
    if l2 == 0:
        return math.hypot(px - ax, py - ay)
    t = max(0.0, min(1.0, ((px - ax) * dx + (py - ay) * dy) / l2))
    return math.hypot(px - (ax + t * dx), py - (ay + t * dy))


def rr_sdf(px, py, x0, y0, x1, y1, r):
    cx, cy = (x0 + x1) / 2, (y0 + y1) / 2
    hx, hy = (x1 - x0) / 2, (y1 - y0) / 2
    qx = abs(px - cx) - hx + r
    qy = abs(py - cy) - hy + r
    return math.hypot(max(qx, 0.0), max(qy, 0.0)) + min(max(qx, qy), 0.0) - r


def circle_sdf(px, py, cx, cy, r):
    return math.hypot(px - cx, py - cy) - r


# ---------- 颜色 ----------

BG_TOP = hex2rgb("#2b3550")
BG_BOT = hex2rgb("#0d1017")
TRACK = (255, 255, 255, 60)
RED = hex2rgb("#ff5c6c")
AMBER = hex2rgb("#f5a524")
GREEN = hex2rgb("#3ecf8e")
CYAN = hex2rgb("#38bdf8")
BLUE = hex2rgb("#4f8cff")


def blend(dst, src):
    """src 覆盖 dst（RGBA float 0-255）"""
    sa = src[3] / 255.0
    if sa <= 0:
        return dst
    da = dst[3] / 255.0
    oa = sa + da * (1 - sa)
    if oa <= 0:
        return (0.0, 0.0, 0.0, 0.0)
    out = []
    for i in range(3):
        out.append((src[i] * sa + dst[i] * da * (1 - sa)) / oa)
    return (out[0], out[1], out[2], oa * 255)


def sample(x, y, kind):
    """归一化坐标 (0..1) 处求色，返回 RGBA float"""
    # 背景圆角方块
    s = rr_sdf(x, y, 0.04, 0.04, 0.96, 0.96, 0.22)
    if s > 0.006:
        return (0.0, 0.0, 0.0, 0.0)
    t = (y - 0.04) / 0.92
    bg = [BG_TOP[i] + (BG_BOT[i] - BG_TOP[i]) * t for i in range(3)]
    px = (bg[0], bg[1], bg[2], 255.0)
    # 内侧细描边
    bs = rr_sdf(x, y, 0.085, 0.085, 0.915, 0.915, 0.19)
    if abs(bs) <= 0.006:
        px = blend(px, (255, 255, 255, 26))

    if kind == "subagent":
        # 三条滑杆：轨道 + 彩色圆点
        rows = [
            (0.36, 0.62, RED),
            (0.50, 0.38, AMBER),
            (0.64, 0.70, GREEN),
        ]
        thick = 0.075
        for ry, kx, kc in rows:
            if dist_seg(x, y, 0.26, ry, 0.74, ry) <= thick / 2:
                px = blend(px, TRACK)
            if circle_sdf(x, y, kx, ry, 0.075) <= 0:
                px = blend(px, (kc[0], kc[1], kc[2], 255.0))
    elif kind == "dashboard":
        # 上升柱组（底线 0.76）
        base = 0.76
        bars = [(0.28, 0.16), (0.44, 0.27), (0.60, 0.39), (0.76, 0.52)]
        w = 0.095
        for bx, h in bars:
            x0, x1 = bx - w / 2, bx + w / 2
            s = rr_sdf(x, y, x0, base - h, x1, base, 0.03)
            if s <= 0:
                # 柱内垂直渐变：青 -> 蓝
                tt = (y - (base - h)) / max(h, 1e-6)
                c = [CYAN[i] + (BLUE[i] - CYAN[i]) * tt for i in range(3)]
                px = blend(px, (c[0], c[1], c[2], 255.0))
    return px


def render(size, kind):
    """返回 RGBA bytearray"""
    out = bytearray(size * size * 4)
    big = size * SS
    acc = [[0.0, 0.0, 0.0, 0.0] for _ in range(size * size)]
    for by in range(big):
        y = (by + 0.5) / big
        for bx in range(big):
            x = (bx + 0.5) / big
            c = sample(x, y, kind)
            ox, oy = bx // SS, by // SS
            idx = oy * size + ox
            a = acc[idx]
            a[0] += c[0]
            a[1] += c[1]
            a[2] += c[2]
            a[3] += c[3]
    n = SS * SS
    for i in range(size * size):
        a = acc[i]
        o = i * 4
        out[o] = min(255, round(a[0] / n))
        out[o + 1] = min(255, round(a[1] / n))
        out[o + 2] = min(255, round(a[2] / n))
        out[o + 3] = min(255, round(a[3] / n))
    return out


def png_bytes(w, h, rgba):
    def chunk(tag, data):
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    raw = b"".join(b"\x00" + bytes(rgba[y * w * 4 : (y + 1) * w * 4]) for y in range(h))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(raw, 9))
        + chunk(b"IEND", b"")
    )


def ico_bytes(pngs):
    """pngs: [(size, png_bytes)]"""
    header = struct.pack("<HHH", 0, 1, len(pngs))
    offset = 6 + 16 * len(pngs)
    entries = b""
    data = b""
    for size, png in pngs:
        dim = size if size < 256 else 0
        entries += struct.pack("<BBBBHHII", dim, dim, 0, 0, 1, 32, len(png), offset)
        data += png
        offset += len(png)
    return header + entries + data


def main():
    out_dir = sys.argv[1] if len(sys.argv) > 1 else "."
    os.makedirs(out_dir, exist_ok=True)
    for kind, prefix in (("subagent", "subagent-control"), ("dashboard", "dashboard")):
        png256 = png_bytes(256, 256, render(256, kind))
        with open(os.path.join(out_dir, f"appicon-{prefix}.png"), "wb") as f:
            f.write(png256)
        sizes = [16, 24, 32, 48, 64, 128, 256]
        pngs = [(s, png_bytes(s, s, render(s, kind))) for s in sizes]
        with open(os.path.join(out_dir, f"icon-{prefix}.ico"), "wb") as f:
            f.write(ico_bytes(pngs))
        print(f"{prefix}: appicon 256px + ico {sizes} -> {out_dir}")


if __name__ == "__main__":
    main()
