#!/usr/bin/env python3
"""Generate every CodaW application icon from assets/logo.svg.

The source of truth for the brand is the single SVG at the repository root. Every
raster icon this application ships is rendered from it here, so that changing the
logo means re-running this script rather than hand-editing a dozen binaries.

Rendering uses macOS' QuickLook (`qlmanage`), which rasterizes the SVG at each
target size, and `iconutil` for the macOS icon set. No third-party tooling is
needed, but the script therefore only runs on macOS; the icons it produces are
committed, so a build on another platform never has to run it.

Usage (from the `editor` directory):  npm run icons
"""

import os
import re
import shutil
import struct
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
EDITOR = os.path.dirname(HERE)
REPO = os.path.dirname(EDITOR)
LOGO = os.path.join(REPO, 'assets', 'logo.svg')

# The dark of the logo plate and the accent, kept here only for the splash screen check.
PLATE = '#17181C'

# macOS draws application icons on a rounded plate inset in a transparent canvas. Apple's
# grid puts an 824pt plate with a 185.4pt corner radius inside a 1024pt canvas.
MAC_CANVAS = 1024
MAC_PLATE = 824
MAC_RADIUS = 185.4

# Sizes electron-builder and Windows expect in an .ico, plus the small set a favicon needs.
ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]
FAVICON_SIZES = [16, 32, 48]
# macOS icon sets are declared as a logical size plus a scale.
ICNS_SIZES = [(16, 1), (16, 2), (32, 1), (32, 2), (128, 1), (128, 2), (256, 1), (256, 2), (512, 1), (512, 2)]


def read_logo():
    """Return the logo's `<g>` glyph block, without the plate it is drawn on."""
    with open(LOGO, encoding='utf-8') as f:
        svg = f.read()
    match = re.search(r'(<g transform="translate\(256,256\) scale\(4\)".*?</g>)', svg, re.S)
    if not match:
        raise SystemExit(f'could not find the glyph group in {LOGO}')
    return svg, match.group(1)


def square_svg(svg, size):
    """The logo as it is: a full-bleed square plate, used for Windows, Linux and the web."""
    return re.sub(r'width="512" height="512"', f'width="{size}" height="{size}"', svg, count=1)


def rounded_svg(glyph, size):
    """The logo on a rounded plate in a transparent canvas, for macOS and in-application use."""
    inset = (MAC_CANVAS - MAC_PLATE) / 2
    # Keep the glyph the same fraction of the plate that it is of the square logo.
    scale = 4 * MAC_PLATE / 512
    glyph = glyph.replace('translate(256,256) scale(4)', f'translate({MAC_CANVAS / 2},{MAC_CANVAS / 2}) scale({scale})')
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{size}" height="{size}" '
        f'viewBox="0 0 {MAC_CANVAS} {MAC_CANVAS}" role="img" aria-label="CodaW logo">\n'
        f'  <rect x="{inset}" y="{inset}" width="{MAC_PLATE}" height="{MAC_PLATE}" rx="{MAC_RADIUS}" fill="{PLATE}"/>\n'
        f'  {glyph}\n'
        f'</svg>\n'
    )


def render(svg_text, size, out_png, workdir):
    """Rasterize `svg_text` at `size`x`size` into `out_png`."""
    svg_path = os.path.join(workdir, f'render-{size}.svg')
    with open(svg_path, 'w', encoding='utf-8') as f:
        f.write(svg_text)
    out_dir = os.path.join(workdir, f'out-{size}')
    shutil.rmtree(out_dir, ignore_errors=True)
    os.makedirs(out_dir)
    subprocess.run(
        ['qlmanage', '-t', '-s', str(size), '-o', out_dir, svg_path],
        check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    rendered = [os.path.join(out_dir, n) for n in os.listdir(out_dir) if n.endswith('.png')]
    if not rendered:
        raise SystemExit(f'QuickLook produced no PNG for size {size}')
    # QuickLook fits the thumbnail into the requested box, which can be a pixel short on
    # non-power-of-two sizes; `sips` pins it to the exact square the icon formats require.
    subprocess.run(
        ['sips', '-z', str(size), str(size), rendered[0], '--out', out_png],
        check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    return out_png


def write_ico(pngs, out_path):
    """Pack PNGs into an .ico. PNG-compressed entries are read by Windows and every browser."""
    entries = []
    for size, path in pngs:
        with open(path, 'rb') as f:
            entries.append((size, f.read()))
    offset = 6 + 16 * len(entries)
    header = struct.pack('<HHH', 0, 1, len(entries))
    directory = b''
    for size, data in entries:
        # A 256px entry is declared as 0: the field is a single byte.
        directory += struct.pack('<BBBBHHII', size % 256, size % 256, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
    with open(out_path, 'wb') as f:
        f.write(header + directory + b''.join(data for _, data in entries))


def ensure_dir(path):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    return path


def main():
    if sys.platform != 'darwin':
        raise SystemExit('this script needs macOS (qlmanage and iconutil); the icons it generates are committed')
    svg, glyph = read_logo()
    work = tempfile.mkdtemp(prefix='codaw-icons-')
    try:
        def square(size):
            return render(square_svg(svg, size), size, os.path.join(work, f'square-{size}.png'), work)

        def rounded(size):
            return render(rounded_svg(glyph, size), size, os.path.join(work, f'rounded-{size}.png'), work)

        written = []

        # Windows launcher and installer icon, and the browser favicon.
        write_ico([(s, square(s)) for s in ICO_SIZES],
                  ensure_dir(os.path.join(EDITOR, 'electron-app', 'resources', 'icon.ico')))
        written.append('electron-app/resources/icon.ico')
        write_ico([(s, square(s)) for s in FAVICON_SIZES],
                  ensure_dir(os.path.join(EDITOR, 'browser-app', 'ico', 'favicon.ico')))
        written.append('browser-app/ico/favicon.ico')

        # Linux launcher icon and the Electron window icon (the AppImage fallback).
        for target in ('electron-app/resources/icons/LinuxLauncherIcons/512x512.png',
                       'electron-app/resources/icons/WindowIcon/512-512.png'):
            shutil.copyfile(square(512), ensure_dir(os.path.join(EDITOR, *target.split('/'))))
            written.append(target)

        # The in-application icon: the welcome tab, the welcome header and the About dialog.
        shutil.copyfile(rounded(512), ensure_dir(os.path.join(
            EDITOR, 'codaw-product', 'src', 'browser', 'icons', 'codaw-512.png')))
        written.append('codaw-product/src/browser/icons/codaw-512.png')

        # macOS icon set.
        iconset = os.path.join(work, 'CodaW.iconset')
        os.makedirs(iconset, exist_ok=True)
        for size, scale in ICNS_SIZES:
            suffix = '' if scale == 1 else '@2x'
            shutil.copyfile(rounded(size * scale), os.path.join(iconset, f'icon_{size}x{size}{suffix}.png'))
        icns = ensure_dir(os.path.join(EDITOR, 'electron-app', 'resources', 'icon.icns'))
        subprocess.run(['iconutil', '-c', 'icns', iconset, '-o', icns], check=True)
        written.append('electron-app/resources/icon.icns')

        for path in written:
            print(f'  {path}')
    finally:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == '__main__':
    main()
