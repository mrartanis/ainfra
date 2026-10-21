"""Run static agent, unprivileged and offline, on pinned distro images."""
import json
import os
from pathlib import Path
import subprocess
import argparse
import uuid

IMAGES = {
    'debian-12': 'debian@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8',
    'debian-13': 'debian@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad',
    'ubuntu-22.04': 'ubuntu@sha256:0a9887931b0e38af2e3487ed39e29436b0535c4f3f52e57745baaf79d1898956',
    'ubuntu-24.04': 'ubuntu@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac',
    'ubuntu-26.04': 'ubuntu@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944',
}
ARM64_IMAGES = {
    'debian-12': 'debian@sha256:991827bb788d78bbc130dcb851913beecff43296360f48838aca7cbdfae4375c',
    'debian-13': 'debian@sha256:a66b19aa552f595b4ab276c6b4f9b24fa4546632c5ecd88cccdced4a6866323a',
    'ubuntu-22.04': 'ubuntu@sha256:88881eb36da489f9ff8746ece8048ca16cc94f80e8cd3ae7e8335d8822631591',
    'ubuntu-24.04': 'ubuntu@sha256:18b33ff3829707f31ada654a963b9ee35f2e6f0241b964bd17a22a9b330cb63d',
    'ubuntu-26.04': 'ubuntu@sha256:7f787d965bc0e4cd42b5f74c013515d6e24ce691c881c81c42e4db59a791e5ce',
}
root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument('distro', nargs='?', choices=list(IMAGES))
parser.add_argument('--arch', choices=['amd64', 'arm64'], default='amd64')
parser.add_argument('--agent', type=Path, default=root / 'bin/infra-agent')
args = parser.parse_args()
agent = args.agent.resolve()
with agent.open('rb') as binary:
    header = binary.read(20)
assert header[:4] == b'\x7fELF' and int.from_bytes(header[18:20], 'little') == {'amd64': 62, 'arm64': 183}[args.arch], 'agent binary architecture mismatch'
if args.arch == 'arm64':
    IMAGES = ARM64_IMAGES
results = []
for name, image in IMAGES.items():
    if args.distro and name != args.distro:
        continue
    state = root / '.local' / 'matrix' / (name + '-' + args.arch)
    state.mkdir(parents=True, exist_ok=True)
    token = state / 'token'
    token.write_text('a' * 64)  # Synthetic offline credential, never used on an API.
    token.chmod(0o600)
    config = dict(node_id=str(uuid.uuid4()), agent_instance_id=str(uuid.uuid4()),
                  server_url='http://127.0.0.1:1', token_file='/state/token',
                  state_dir='/state/runtime', collectors=['os', 'packages', 'network', 'disks'])
    (state / 'agent.json').write_text(json.dumps(config))
    base = ['docker', 'run', '--rm', '--platform', 'linux/' + args.arch, '--network', 'none', '--read-only',
            '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
            '--user', f'{os.getuid()}:{os.getgid()}']
    command = base + ['-v', f'{agent}:/infra-agent:ro',
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
    assert report['collectors']['os']['data']['arch'] == args.arch
    assert report['collectors']['disks']['status'] == 'ok'
    result = dict(image=name, architecture=args.arch, digest=image, packages=len(got),
                  os=report['collectors']['os']['data']['version'], status='passed')
    results.append(result)
    print(json.dumps(result))
(root / '.local' / ('matrix-results-' + args.arch + '.json')).write_text(json.dumps(results, indent=2))
