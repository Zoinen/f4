#!/usr/bin/env python3
"""Export the macOS FFmpeg recipe from the pinned ConanCenter source recipe."""

from pathlib import Path
import shutil
import subprocess
import tempfile


UPSTREAM = "ffmpeg/7.1.5#6b4370298de380e88bf6f179b7373a46"
PATCH = "ffmpeg-7.1.5-videotoolbox-dynamic-hdr.patch"


def patch_recipe(recipe: str) -> str:
    changes = (
        ("    package_type = \"library\"\n",
         f'    exports_sources = "{PATCH}"\n'),
        ("        get(self, **self.conan_data[\"sources\"][self.version], strip_root=True)\n",
         '        from conan.tools.files import patch\n'
         f'        patch(self, patch_file=os.path.join(self.export_sources_folder, "{PATCH}"))\n'),
    )
    for anchor, addition in changes:
        if recipe.count(anchor) != 1:
            raise ValueError(f"unexpected FFmpeg recipe near {anchor.strip()!r}")
        if anchor + addition not in recipe:
            recipe = recipe.replace(anchor, anchor + addition)
    return recipe


def main() -> None:
    cache_path = ["conan", "cache", "path", UPSTREAM]
    cached = subprocess.run(cache_path, text=True, capture_output=True)
    if cached.returncode:
        subprocess.run(["conan", "download", UPSTREAM, "--only-recipe", "-r", "conancenter"],
                       check=True)
        cached = subprocess.run(cache_path, text=True, capture_output=True, check=True)
    upstream = Path(cached.stdout.strip())
    with tempfile.TemporaryDirectory(prefix="f4-ffmpeg-recipe-") as directory:
        target = Path(directory)
        shutil.copy2(upstream / "conandata.yml", target)
        shutil.copy2(Path(__file__).parent / "patches" / PATCH, target)
        recipe = (upstream / "conanfile.py").read_text(encoding="utf-8")
        (target / "conanfile.py").write_text(patch_recipe(recipe), encoding="utf-8")
        subprocess.run(["conan", "export", directory, "--name=ffmpeg", "--version=7.1.5",
                        "--user=f4", "--channel=macos"], check=True)


if __name__ == "__main__":
    main()
