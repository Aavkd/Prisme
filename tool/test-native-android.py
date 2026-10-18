"""Run the native test on an existing emulator, staging inputs only in its app sandbox."""
from pathlib import Path
import shutil
import subprocess
import sys
import time

root = Path(__file__).resolve().parent.parent
device = sys.argv[1] if len(sys.argv) > 1 else 'emulator-5554'
if not device.startswith('emulator-') or not device[9:].isdigit():
    raise SystemExit('This helper only targets an explicitly named Android emulator.')
adb = shutil.which('adb')
flutter = shutil.which('flutter')
if not adb or not flutter:
    raise SystemExit('adb and Flutter are required.')
base = [adb, '-s', device]
private = base + ['shell', 'run-as', 'dev.mantara.prisme_client']
files = ['files/phase1/bootstrap-secret', 'files/phase1/pki/ca.pem', 'files/phase1/awaiting-input', 'files/phase1/capture-ready', 'files/phase1/capture-copied']

def quiet(args, **kwargs):
    return subprocess.run(args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15, **kwargs)

quiet(private + ['rm', '-f', *files])
subprocess.run(base + ['reverse', 'tcp:8443', 'tcp:8443'], check=True)
process = subprocess.Popen([flutter, 'test', 'integration_test/local_backend_test.dart', '-d', device], cwd=root/'apps/prisme_client')
staged = False
captured = False
try:
    while process.poll() is None:
        if not staged and quiet(private + ['test', '-f', 'files/phase1/awaiting-input']).returncode == 0:
            quiet(private + ['mkdir', '-p', 'files/phase1/pki'], check=True)
            # CA first, secret last: the test uses secret presence as its ready signal.
            for source, target in [('.dev/pki/ca.pem', files[1]), ('.dev/bootstrap-secret', files[0])]:
                with (root/source).open('rb') as stream:
                    quiet(private + ['tee', target], stdin=stream, check=True)
            staged = True
        if staged and not captured and quiet(private + ['test', '-f', 'files/phase1/capture-ready']).returncode == 0:
            with (root/'.dev/phase1-android.png').open('wb') as image:
                subprocess.run(base + ['exec-out', 'run-as', 'dev.mantara.prisme_client', 'cat', 'files/phase1/phase1-android.png'], stdout=image, check=True)
            quiet(private + ['tee', 'files/phase1/capture-copied'], input=b'copied', check=True)
            captured = True
        time.sleep(1)
    result = process.returncode
    if result == 0 and not captured:
        raise RuntimeError('Native test passed but no screenshot was retrieved.')
finally:
    quiet(private + ['rm', '-f', *files])
    if process.poll() is None:
        process.terminate()
raise SystemExit(result)
