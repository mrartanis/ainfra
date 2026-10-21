"""Test the example wrapper using a fake backup executable, never a real repository."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

hook = Path(__file__).resolve().parent.parent / 'packaging/examples/restic-backup-hook.sh'
with tempfile.TemporaryDirectory(prefix='infra-hook-') as directory:
    root = Path(directory)
    executable = root / 'restic'
    executable.write_text('#!/bin/sh\nexit "${FIXTURE_EXIT:-0}"\n')
    executable.chmod(0o700)
    output = root / 'check.json'
    for code in [0, 1, 3]:
        result = subprocess.run([str(hook), str(output), 'nightly', str(executable), 'backup', '/fixture'], env=dict(os.environ, FIXTURE_EXIT=str(code)))
        assert result.returncode == code
        check = json.loads(output.read_text())
        assert check['data']['last_attempt_status'] == ('success' if code == 0 else 'error')
        assert bool(check['data']['last_success_at']) == (code == 0)
        assert 'last_restore_test_at' not in check['data']
    assert subprocess.run([str(hook), str(output), 'nightly', str(executable), 'snapshots']).returncode == 64
print('Backup hook smoke passed: success/error/incomplete, no invented restore result, snapshots rejected')
