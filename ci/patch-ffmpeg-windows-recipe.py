#!/usr/bin/env python3
"""Export the pinned FFmpeg recipe with pkg-config metadata for MSVC builds."""

from pathlib import Path
import shutil
import subprocess
import tempfile


UPSTREAM = "ffmpeg/7.1.5#6b4370298de380e88bf6f179b7373a46"
ANCHOR = '            env.vars(self).save_script("conanautotoolsdeps_cl_workaround")\n'
MARKER = "# f4: generate pkg-config metadata for the MSVC FFmpeg configure step"
PATCH = (
    ANCHOR
    + f"            {MARKER}\n"
    + "            # Keep Conan's MSVC compile and link flags above while also\n"
    + "            # generating the .pc files required by FFmpeg's configure\n"
    + "            # probes for dependencies such as OpenJPEG.\n"
    + "            deps = PkgConfigDeps(self)\n"
    + "            deps.generate()\n"
)


def patch_recipe(recipe: str) -> str:
    if MARKER in recipe:
        return recipe
    if recipe.count(ANCHOR) != 1:
        raise ValueError("unexpected FFmpeg recipe: MSVC AutotoolsDeps anchor is absent or ambiguous")
    if "PkgConfigDeps" not in recipe:
        raise ValueError("unexpected FFmpeg recipe: PkgConfigDeps is not imported")
    return recipe.replace(ANCHOR, PATCH)


def main() -> None:
    cached = subprocess.run(
        ["conan", "cache", "path", UPSTREAM], text=True, capture_output=True
    )
    if cached.returncode:
        subprocess.run(
            ["conan", "download", UPSTREAM, "--only-recipe", "-r", "conancenter"],
            check=True,
        )
        cached = subprocess.run(
            ["conan", "cache", "path", UPSTREAM],
            text=True,
            capture_output=True,
            check=True,
        )
    upstream = Path(cached.stdout.strip())
    with tempfile.TemporaryDirectory(prefix="f4-ffmpeg-windows-recipe-") as directory:
        target = Path(directory)
        shutil.copy2(upstream / "conandata.yml", target)
        recipe = (upstream / "conanfile.py").read_text(encoding="utf-8")
        (target / "conanfile.py").write_text(patch_recipe(recipe), encoding="utf-8")
        subprocess.run(
            [
                "conan",
                "export",
                directory,
                "--name=ffmpeg",
                "--version=7.1.5",
                "--user=f4",
                "--channel=windows",
            ],
            check=True,
        )


if __name__ == "__main__":
    main()
