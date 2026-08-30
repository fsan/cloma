#!/usr/bin/env python3
"""Generate the cloma app icons.

Produces:
  build/icon.png            - 1024x1024 coloured app icon (for the .app bundle)
  build/trayIconTemplate.png        - 22x22  black "C" template image (menu bar)
  build/trayIconTemplate@2x.png     - 44x44  black "C" template image (menu bar retina)

The tray icons are solid black with an alpha channel so macOS can render them
as template images that adapt to the light/dark menu bar.
"""

import os
from PIL import Image, ImageDraw, ImageFont

BUILD_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "build")


def _load_font(size):
    candidates = [
        "/home/agent/.local/share/junie/versions/2777.6/junie-app/lib/runtime/lib/fonts/Inter-SemiBold.otf",
        "/home/agent/.local/share/junie/versions/2777.6/junie-app/lib/runtime/lib/fonts/JetBrainsMono-Bold.ttf",
        "/System/Library/Fonts/Helvetica.ttc",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    ]
    for path in candidates:
        if os.path.exists(path):
            try:
                return ImageFont.truetype(path, size)
            except Exception:
                continue
    return ImageFont.load_default()


def make_app_icon(path):
    size = 1024
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Rounded-square background with a vertical gradient (indigo -> blue).
    radius = size // 5
    for y in range(size):
        t = y / size
        r = int(79 + (29 - 79) * t)
        g = int(70 + (78 - 70) * t)
        b = int(229 + (216 - 229) * t)
        draw.rectangle([(0, y), (size, y + 1)], fill=(r, g, b, 255))

    # Mask to a rounded square.
    mask = Image.new("L", (size, size), 0)
    mdraw = ImageDraw.Draw(mask)
    mdraw.rounded_rectangle([(0, 0), (size, size)], radius=radius, fill=255)
    img.putalpha(mask)

    # Draw a white capital "C" centred in the square.
    font = _load_font(int(size * 0.72))
    text = "C"
    bbox = draw.textbbox((0, 0), text, font=font)
    tw = bbox[2] - bbox[0]
    th = bbox[3] - bbox[1]
    tx = (size - tw) / 2 - bbox[0]
    ty = (size - th) / 2 - bbox[1] - int(size * 0.02)
    draw.text((tx, ty), text, font=font, fill=(255, 255, 255, 255))

    img.save(path)
    print(f"Wrote {path}")


def make_tray_icon(path, px):
    img = Image.new("RGBA", (px, px), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    font = _load_font(int(px * 0.82))
    text = "C"
    bbox = draw.textbbox((0, 0), text, font=font)
    tw = bbox[2] - bbox[0]
    th = bbox[3] - bbox[1]
    tx = (px - tw) / 2 - bbox[0]
    ty = (px - th) / 2 - bbox[1] - int(px * 0.02)
    draw.text((tx, ty), text, font=font, fill=(0, 0, 0, 255))
    img.save(path)
    print(f"Wrote {path}")


def main():
    os.makedirs(BUILD_DIR, exist_ok=True)
    make_app_icon(os.path.join(BUILD_DIR, "icon.png"))
    make_tray_icon(os.path.join(BUILD_DIR, "trayIconTemplate.png"), 22)
    make_tray_icon(os.path.join(BUILD_DIR, "trayIconTemplate@2x.png"), 44)


if __name__ == "__main__":
    main()