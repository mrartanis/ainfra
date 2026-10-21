"""Run static agent, unprivileged and offline, on pinned distro images."""
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid

IMAGES = {
    'debian-12': 'debian@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8',
    'debian-13': 'debian@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad',
    'ubuntu-22.04': 'ubuntu@sha256:0a9887931b0e38af2e3487ed39e29436b0535c4f3f52e57745baaf79d1898956',
    'ubuntu-24.04': 'ubuntu@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac',
    'ubuntu-26.04': 'ubuntu@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944',
}
root = Path(__file__).resolve().parent.parent
results = []
for name, image in IMAGES.items():
    if len(sys.argv) > 1 and name != sys.argv[1]:
        continue
    state = root / '.local' / 'matrix' / name
    state.mkdir(parents=True, exist_ok=True)
    token = state / 'token'
    token.write_text('a' * 64)  # Synthetic offline credential, never used on an API.
    token.chmod(0o600)
    config = dict(node_id=str(uuid.uuid4()), agent_instance_id=str(uuid.uuid4()),
                  server_url='http://127.0.0.1:1', token_file='/state/token',
                  state_dir='/state/runtime', collectors=['os', 'packages', 'network', 'disks'])
    (state / 'agent.json').write_text(json.dumps(config))
    base = ['docker', 'run', '--rm', '--network', 'none', '--read-only',
            '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
            '--user', f'{os.getuid()}:{os.getgid()}']
    command = base + ['-v', f'{root / "bin/infra-agent"}:/infra-agent:ro',
                      '-v', f'{state}:/state', image, '/infra-agent', 'collect',
                      '--config', '/state/agent.json', '--json']
    collected = subprocess.run(command, check=True, capture_output=True, timeout=180)
    report = json.loads(collected.stdout)
    (state / 'report.json').write_bytes(collected.stdout)
    packages = report['collectors']['packages']
    assert packages['status'] == 'ok' and packages['complete'], (name, packages)
    actual = subprocess.run(base + [image, 'dpkg-query', '-W',
                                    '-f=${Package}\t${Architecture}\t${Version}\t${source:Package}\t${source:Version}\t${db:Status-Status}\n'],
                            check=True, capture_output=True, text=True, timeout=30)
    expected = sorted(tuple(line.split('\t')[:5]) for line in actual.stdout.splitlines()
                      if line.split('\t')[-1] not in ('config-files', 'not-installed'))
    got = sorted((p['name'], p['arch'], p['version'], p['source'], p['source_version'])
                 for p in packages['data']['packages'])
    assert expected == got, f'{name}: package/source inventory differs from dpkg'
    assert report['collectors']['os']['status'] == 'ok'
    assert report['collectors']['disks']['status'] == 'ok'
    result = dict(image=name, digest=image, packages=len(got),
                  os=report['collectors']['os']['data']['version'], status='passed')
    results.append(result)
    print(json.dumps(result))
(root / '.local' / 'matrix-results.json').write_text(json.dumps(results, indent=2))
