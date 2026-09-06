"""Derive the two v0.2 review views from the shared home-screen fragment."""
from pathlib import Path
import argparse

parser = argparse.ArgumentParser()
parser.add_argument('inline_directory', type=Path)
args = parser.parse_args()
design = Path(__file__).resolve().parent
source = (design / 'home.html').read_text(encoding='utf-8')
args.inline_directory.mkdir(parents=True, exist_ok=True)

for name, device, theme, label in (
    ('home-mobile', 'android', 'dark', 'Prisme · Accueil Android'),
    ('home-matrix', 'windows', 'matrix', 'Prisme · Thème Terminal / Matrix'),
):
    fragment = source.replace('prisme-home', f'prisme-{name}')
    fragment = fragment.replace("device:'windows'", f"device:'{device}'", 1)
    fragment = fragment.replace("theme:'system'", f"theme:'{theme}'", 1)
    fragment = fragment.replace('data-device="windows"', f'data-device="{device}"', 1)
    fragment = fragment.replace('Prisme · Home screen study', label)
    (design / f'{name}.html').write_text(fragment, encoding='utf-8')
    (args.inline_directory / f'prisme-{name}.html').write_text(fragment, encoding='utf-8')
    print(f'{name}: {device}, {theme}')
