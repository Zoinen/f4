#!/usr/bin/env python3
"""Expose generated Conan pkg-config files to Windows FFmpeg builds."""

from pathlib import Path
import shutil
import subprocess
import tempfile


UPSTREAM = "ffmpeg/7.1.5#6b4370298de380e88bf6f179b7373a46"
ANCHOR = "        autotools = Autotools(self)\n        autotools.configure()\n"
MARKER = "# f4: expose Conan-generated pkg-config files to the Windows FFmpeg probe"
PATCH = (
    ANCHOR
    .replace(
        "        autotools.configure()\n",
        "        if is_msvc(self):\n"
        f"            {MARKER}\n"
        "            # PkgConfigDeps writes .pc files into this folder but does not\n"
        "            # add it to the process search path used by FFmpeg configure.\n"
        "            if self.options.with_openjpeg:\n"
        "                openjpeg_pc = os.path.join(self.generators_folder, \"libopenjp2.pc\")\n"
        "                if not os.path.isfile(openjpeg_pc):\n"
        "                    raise RuntimeError(f\"Conan did not generate required pkg-config file: {openjpeg_pc}\")\n"
        "            generated_pkg_config_path = os.path.abspath(self.generators_folder)\n"
        "            existing_pkg_config_path = os.environ.get(\"PKG_CONFIG_PATH\")\n"
        "            pkg_config_paths = [generated_pkg_config_path]\n"
        "            if existing_pkg_config_path:\n"
        "                pkg_config_paths.append(existing_pkg_config_path)\n"
        "            os.environ[\"PKG_CONFIG_PATH\"] = os.pathsep.join(pkg_config_paths)\n"
        "        autotools.configure()\n",
    )
)


def patch_recipe(recipe: str) -> str:
    if MARKER in recipe:
        return recipe
    if recipe.count(ANCHOR) != 1:
        raise ValueError("unexpected FFmpeg recipe: Autotools.configure anchor is absent or ambiguous")
    if "PkgConfigDeps(self)" not in recipe or "deps.generate()" not in recipe:
        raise ValueError("unexpected FFmpeg recipe: PkgConfigDeps files are not generated")
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
