"""Exact capture comparison, including RGB changes with unchanged alpha."""
from PIL import ImageChops


def compare_images(first, second):
    if first.size != second.size:
        raise ValueError('Capture dimensions differ')
    difference = ImageChops.difference(first.convert('RGBA'), second.convert('RGBA'))
    rgb = difference.convert('RGB').getbbox()
    alpha = difference.getchannel('A').getbbox()
    bounds = [b for b in (rgb, alpha) if b is not None]
    bounding_box = None if not bounds else (
        min(b[0] for b in bounds), min(b[1] for b in bounds),
        max(b[2] for b in bounds), max(b[3] for b in bounds))
    pixels = difference.tobytes()
    count = sum(any(pixels[i:i+4]) for i in range(0, len(pixels), 4))
    return {'changedPixels': count, 'bounds': bounding_box}
