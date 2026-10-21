"""Opt-in archive install/restart/restore smoke in a clean, offline Debian container."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import uuid

root = Path(__file__).resolve().parent.parent
image = 'debian@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad'
archive = root / 'dist/infra-0.1.0-linux-amd64.tar.gz'
name = 'infra-release-' + uuid.uuid4().hex[:12]
with tempfile.TemporaryDirectory(prefix='infra-release-') as directory:
    config = Path(directory)
    config.chmod(0o755)
    def write(name, value):
        (config / name).write_text(json.dumps(value))
    write('cli.json', {'endpoint': 'http://127.0.0.1:18080', 'token_file': '/tmp/state/admin.token'})
    write('restored.json', {'endpoint': 'http://127.0.0.1:18082', 'token_file': '/tmp/state/admin.token'})
    def execute(*args, check=True):
        result = subprocess.run(['docker', 'exec', name, *args], text=True, capture_output=True, timeout=180)
        if check and result.returncode:
            raise RuntimeError('container command failed: ' + result.stderr[:1000])
        return result
    def cli(*args, restored=False):
        return execute('/tmp/install/bin/infractl', '--config', '/config/restored.json' if restored else '/config/cli.json', *args)
    def start(db, port, pid):
        subprocess.run(['docker', 'exec', '-d', name, 'sh', '-c',
                        f'echo $$ > /tmp/{pid}; exec /tmp/install/bin/infra-server serve --db {db} --listen 127.0.0.1:{port} --feeds=false >/tmp/{pid}.log 2>&1'], check=True)
        for _ in range(100):
            result = execute('/tmp/install/bin/infractl', '--config', '/config/restored.json' if port == 18082 else '/config/cli.json', 'me', check=False)
            if result.returncode == 0:
                return
            time.sleep(.1)
        raise RuntimeError('server did not become ready')
    try:
        subprocess.run(['docker', 'run', '-d', '--name', name, '--network', 'none', '--read-only',
                        '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--user', f'{os.getuid()}:{os.getgid()}',
                        '--tmpfs', '/tmp:exec,mode=1777', '-v', f'{archive}:/release.tar.gz:ro', '-v', f'{config}:/config:ro',
                        image, 'sleep', '600'], check=True, capture_output=True)
        execute('sh', '-c', 'mkdir /tmp/install /tmp/state; tar -xzf /release.tar.gz --strip-components=1 -C /tmp/install')
        for tool in ['go', 'python3', 'node', 'docker']:
            assert execute('sh', '-c', 'command -v ' + tool, check=False).returncode != 0
        server = '/tmp/install/bin/infra-server'
        db = '/tmp/state/infra.db'
        execute(server, 'bootstrap', '--db', db, '--output', '/tmp/state/admin.token')
        node = json.loads(execute(server, 'create-node', '--db', db, '--name', 'archive-smoke').stdout)
        instance = str(uuid.uuid4())
        execute(server, 'bootstrap', '--db', db, '--role', 'agent', '--node', node['id'], '--instance', instance, '--output', '/tmp/state/agent.token')
        write('agent.json', {'node_id': node['id'], 'agent_instance_id': instance, 'server_url': 'http://127.0.0.1:18080',
                            'token_file': '/tmp/state/agent.token', 'state_dir': '/tmp/state/agent', 'collectors': ['os', 'packages', 'disks']})
        start(db, 18080, 'server.pid')
        execute('/tmp/install/bin/infra-agent', 'run', '--config', '/config/agent.json')
        observed = json.loads(cli('nodes', 'show', node['id']).stdout)
        assert observed['sequence'] == 1 and observed['observations']['packages']['data']['packages']
        write('doc.json', {'title': 'Archive restore evidence', 'kind': 'runbook', 'body': 'Unique archive smoke phrase', 'nodes': [node['id']]})
        doc = json.loads(cli('docs', 'put', '--file', '/config/doc.json').stdout)
        execute(server, 'backup', '--db', db, '--output', '/tmp/state/backup.db')
        execute('sh', '-c', 'kill -TERM "$(cat /tmp/server.pid)"')
        for _ in range(100):
            if execute('sh', '-c', 'kill -0 "$(cat /tmp/server.pid)"', check=False).returncode != 0:
                break
            time.sleep(.1)
        start(db, 18080, 'server.pid')
        assert json.loads(cli('nodes', 'show', node['id']).stdout)['sequence'] == 1
        execute('/tmp/install/bin/infra-agent', 'run', '--config', '/config/agent.json')
        assert json.loads(cli('nodes', 'show', node['id']).stdout)['sequence'] == 2
        start('/tmp/state/backup.db', 18082, 'restored.pid')
        assert json.loads(cli('nodes', 'show', node['id'], restored=True).stdout)['sequence'] == 1
        assert json.loads(cli('docs', 'get', doc['id'], restored=True).stdout)['body'] == 'Unique archive smoke phrase'
        assert json.loads(cli('docs', 'search', 'Unique archive', '--literal', restored=True).stdout)
        cli('export', '--output', '/tmp/export', restored=True)
        execute('sh', '-c', 'test -n "$(find /tmp/export -name "*.md" -print -quit)"')
        execute('sh', '-c', 'kill -TERM "$(cat /tmp/server.pid)" "$(cat /tmp/restored.pid)"')
        execute('rm', '-r', '/tmp/install')
        execute('test', '-s', db)  # Uninstall leaves state and backup intact.
        execute('test', '-s', '/tmp/state/backup.db')
        print('Release smoke passed: clean non-root/offline install, real agent, restart, active backup/restore, auth, search, export, uninstall preserves data')
    finally:
        subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
