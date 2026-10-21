"""Opt-in archive install/restart/restore smoke in a clean, offline Debian container."""
import json
import argparse
import os
from pathlib import Path
import subprocess
import tempfile
import time
import uuid

root = Path(__file__).resolve().parent.parent
image = 'debian@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad'
parser = argparse.ArgumentParser()
parser.add_argument('--arch', choices=['amd64', 'arm64'], default='amd64')
arch = parser.parse_args().arch
if arch == "arm64":
    image = "debian@sha256:a66b19aa552f595b4ab276c6b4f9b24fa4546632c5ecd88cccdced4a6866323a"
archive = root / ('dist/infra-0.1.0-linux-' + arch + '.tar.gz')
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
        subprocess.run(['docker', 'run', '-d', '--platform', 'linux/' + arch, '--name', name, '--network', 'none', '--read-only',
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
        # Non-UTC recurrence must work even in the minimal image without tzdata.
        observed['payment'] = {'next_due': '2026-03-31T10:00:00+02:00', 'interval': 1,
                               'unit': 'months', 'timezone': 'Europe/Berlin', 'note': 'calendar smoke'}
        write('payment-node.json', observed)
        scheduled = json.loads(cli('nodes', 'put', '--file', '/config/payment-node.json',
                                   '--if-revision', str(observed['revision'])).stdout)
        paid = json.loads(cli('request', 'POST', 'nodes/' + node['id'] + '/pay',
                              '--if-revision', str(scheduled['revision'])).stdout)
        assert paid['payment']['next_due'] == '2026-04-30T10:00:00+02:00'
        assert not paid['payment']['last_paid'].startswith('0001-')
        write('doc.json', {'title': 'Archive restore evidence', 'kind': 'runbook', 'body': 'Unique archive smoke phrase', 'nodes': [node['id']]})
        doc = json.loads(cli('docs', 'put', '--file', '/config/doc.json').stdout)
        # Walk through the shipped skill using actual binaries and role-scoped tokens.
        for role in ['reader', 'editor']:
            execute(server, 'bootstrap', '--db', db, '--role', role, '--output', '/tmp/state/' + role + '.token')
            write(role + '.json', {'endpoint': 'http://127.0.0.1:18080', 'token_file': '/tmp/state/' + role + '.token'})
        def scoped(role, *args, check=True):
            return execute('/tmp/install/bin/infractl', '--config', '/config/' + role + '.json', *args, check=check)
        assert json.loads(scoped('reader', 'me').stdout)['role'] == 'reader'
        for args in [('attention',), ('feeds', 'status'), ('packages', 'list', node['id']), ('findings', 'list', '--node', node['id'])]:
            json.loads(scoped('reader', *args).stdout)
        context = scoped('reader', 'context', node['id'], '--format', 'markdown').stdout
        assert 'UNTRUSTED DATA' in context and 'Unique archive smoke phrase' in context
        write('plan.json', {'title': 'Skill walkthrough fixture', 'kind': 'maintenance_plan', 'nodes': [node['id']],
                            'body': 'Disposable acceptance fixture. Inspect current inventory and vendor evidence. Feeds are absent here: CVE coverage unknown. No updates authorized or executed. Verify a real backup and restore before proposing changes; rollback availability unknown.'})
        assert scoped('reader', 'docs', 'put', '--file', '/config/plan.json', check=False).returncode != 0
        plan = json.loads(scoped('editor', 'docs', 'put', '--file', '/config/plan.json').stdout)
        assert plan['status'] == 'draft'
        assert scoped('editor', 'docs', 'approve', plan['id'], '--if-revision', '1', check=False).returncode != 0
        assert json.loads(cli('docs', 'approve', plan['id'], '--if-revision', '1').stdout)['status'] == 'approved'
        assert scoped('editor', 'docs', 'complete', plan['id'], '--if-revision', '1', check=False).returncode != 0
        assert json.loads(cli('docs', 'complete', plan['id'], '--if-revision', '1').stdout)['status'] == 'completed'
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
        print(arch + ' release smoke passed: clean non-root/offline install, real agent, restart, active backup/restore, auth, role-scoped skill walkthrough, search, export, uninstall preserves data')
    finally:
        subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
