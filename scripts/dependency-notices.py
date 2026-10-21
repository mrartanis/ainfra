"""Copy dependency licenses into a release tree; no network or install actions."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

go = os.environ.get('GO', 'go')
destination = Path(sys.argv[1]) / 'third-party-licenses'
destination.mkdir(parents=True, exist_ok=True)
raw = subprocess.check_output([go, 'list', '-m', '-json', 'all'], text=True)
decoder = json.JSONDecoder()
modules = []
while raw.strip():
    item, end = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[end:]
    if not item.get('Main'):
        modules.append(item)
index = ['Third-party dependency notices', '', 'Versions are pinned in go.mod/go.sum. These licenses apply to their respective dependencies.', '']
for module in modules:
    root = Path(module['Dir'])
    target = destination / (module['Path'].replace('/', '_') + '@' + module['Version'])
    files = [p for p in root.iterdir() if p.is_file() and p.name.lower().startswith(('license', 'copying', 'notice', 'copyright', 'authors'))]
    if not any(p.name.lower().startswith(('license', 'copying')) for p in files):
        raise SystemExit('Missing license for ' + module['Path'])
    target.mkdir(exist_ok=True)
    for path in files:
        shutil.copy2(path, target / path.name)
    index.append(module['Path'] + ' ' + module['Version'])
goroot = Path(subprocess.check_output([go, 'env', 'GOROOT'], text=True).strip())
shutil.copy2(goroot / 'LICENSE', destination / 'Go-LICENSE')
(destination / 'INDEX.txt').write_text('\n'.join(index) + '\n')
