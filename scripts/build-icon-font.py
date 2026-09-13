# Copyright 2026, Quasar
# SPDX-License-Identifier: Apache-2.0
#
# Replaces the legacy wave-logo glyphs in the custom icon font
# (public/fontawesome/webfonts/custom-icons.*) with the Quasar mark traced in
# assets/quasar-logo.svg. Workspaces keep storing "custom@wave-logo-solid", so
# every existing workspace shows the Quasar logo after rebuilding the font.
#
# Usage: python scripts/build-icon-font.py

import re
from pathlib import Path

from fontTools.misc.transform import Transform
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.cu2quPen import Cu2QuPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.svgLib.path import parse_path
from fontTools.ttLib import TTFont

REPO = Path(__file__).resolve().parent.parent
FONT_WOFF2 = REPO / "public" / "fontawesome" / "webfonts" / "custom-icons.woff2"
FONT_TTF = REPO / "public" / "fontawesome" / "webfonts" / "custom-icons.ttf"
LOGO_SVG = REPO / "assets" / "quasar-logo.svg"

# uniE002 = wave-logo-solid, uniE003 = wave-logo-outline.
GLYPH_NAMES = ["uniE002", "uniE003"]

# Optical center of the line box (hhea ascent 448 / descent -64) and a mark
# that fills the em height, so the logo reads as large as the old wordmark.
EM = 512
CENTER_X = EM // 2
CENTER_Y = 192
TARGET_HEIGHT = 448
ADVANCE = EM


def main() -> None:
    d = re.search(r'd="([^"]+)"', LOGO_SVG.read_text(encoding="utf-8")).group(1)
    bounds = BoundsPen(None)
    parse_path(d, bounds)
    x_min, y_min, x_max, y_max = bounds.bounds
    width, height = x_max - x_min, y_max - y_min

    scale = TARGET_HEIGHT / height
    x0 = CENTER_X - (width * scale) / 2
    y0 = CENTER_Y - TARGET_HEIGHT / 2
    transform = Transform(scale, 0, 0, -scale, x0 - x_min * scale, y0 + y_max * scale)

    font = TTFont(str(FONT_WOFF2))
    glyf = font["glyf"]
    for name in GLYPH_NAMES:
        pen = TTGlyphPen(None)
        parse_path(d, TransformPen(Cu2QuPen(pen, 1.0), transform))
        glyph = pen.glyph()
        glyf[name] = glyph
        glyph.recalcBounds(glyf)
        font["hmtx"][name] = (ADVANCE, glyph.xMin)
        print(f"{name}: bbox {(glyph.xMin, glyph.yMin, glyph.xMax, glyph.yMax)} advance {ADVANCE}")

    font.flavor = "woff2"
    font.save(FONT_WOFF2)
    font.flavor = None
    font.save(FONT_TTF)
    print(f"wrote {FONT_WOFF2.name} and {FONT_TTF.name}")


if __name__ == "__main__":
    main()
