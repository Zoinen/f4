import unittest
from PIL import Image
from render_image_compare import compare_images


class CaptureComparisonTests(unittest.TestCase):
    def test_unchanged_alpha_does_not_hide_rgb_difference(self):
        a = Image.new('RGBA', (2, 2), (10, 20, 30, 255))
        b = a.copy()
        b.putpixel((1, 0), (11, 20, 30, 255))
        self.assertEqual(compare_images(a, b), {'changedPixels': 1, 'bounds': (1, 0, 2, 1)})

    def test_alpha_only_difference_is_preserved(self):
        a = Image.new('RGBA', (1, 1), (10, 20, 30, 255))
        b = Image.new('RGBA', (1, 1), (10, 20, 30, 254))
        self.assertEqual(compare_images(a, b)['changedPixels'], 1)

    def test_identical_capture(self):
        a = Image.new('RGB', (3, 2), 'red')
        self.assertEqual(compare_images(a, a), {'changedPixels': 0, 'bounds': None})

    def test_different_sizes_are_rejected(self):
        with self.assertRaises(ValueError):
            compare_images(Image.new('RGB', (1, 1)), Image.new('RGB', (2, 1)))


if __name__ == '__main__':
    unittest.main()
