package inventory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestObservationValidation(t *testing.T) {
	valid := Snapshot{Protocol: "1", Source: "agent", ObservedAt: time.Now().UTC(), Host: Host{Hostname: "fixture", CPUs: 4}}
	if err := valid.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*Snapshot){func(s *Snapshot) { s.ObservedAt = time.Now().Add(5 * time.Minute) }, func(s *Snapshot) { s.ObservedAt = time.Now().Add(-25 * time.Hour) }, func(s *Snapshot) { s.Host.Hostname = "line\nbreak" }, func(s *Snapshot) { s.Host.CPUs = 0 }, func(s *Snapshot) { s.Containers = []Container{{ID: "same"}, {ID: "same"}} }, func(s *Snapshot) { s.Host.MemoryBytes = new(int64(-1)) }} {
		invalid := valid
		modify(&invalid)
		if invalid.Validate(time.Now()) == nil {
			t.Fatal("invalid observation accepted")
		}
	}
}
func TestSSHArgumentBoundary(t *testing.T) {
	for _, alias := range []string{"-oProxyCommand=evil", "host;echo injected", "user@host", "host\ncommand"} {
		if _, err := Collect(context.Background(), alias, ""); err == nil || !strings.Contains(err.Error(), "invalid SSH alias") {
			t.Fatal("unsafe SSH alias accepted")
		}
	}
}
func TestCollectorExcludesSecretsAndReportsDockerFailure(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		python, err = exec.LookPath("python3")
	}
	if err != nil {
		t.Skip("Python unavailable")
	}
	dir := t.TempDir()
	// Test the real Python program with a controlled subprocess adapter. Fixtures
	// include secrets outside the selected fields and Docker details absent on one container.
	shim := `import subprocess, json
class FakeProcess:
    def __init__(self,args,**kwargs):
        import io
        assert args[0]=='docker'
        assert 'Env' not in ' '.join(args) and '.Config' not in ' '.join(args)
        if args[1]=='ps':
            data={'id':'id-a','name':'backend','image':'app:v1','state':'running','ports':'443/tcp','networks':'edge','project':'sounding-lines','service':'backend','config_files':'/opt/compose.yml'}
        elif args[1]=='inspect': data={'id':'id-a','image_id':'sha256:one','health':'not_configured'}
        elif args[1]=='image': data={'id':'sha256:one','tags':['app:v1'],'digests':[]}
        else: data={'name':'edge','driver':'bridge','project':'edge'}
        self.stdout=io.BytesIO((json.dumps(data)+'\n').encode())
    def __enter__(self): return self
    def __exit__(self,*args): pass
    def wait(self,**kwargs): return 0
    def poll(self): return 0
subprocess.Popen=FakeProcess
`
	path := filepath.Join(dir, "collect.py")
	os.WriteFile(path, []byte(shim+collector), 0600)
	out, err := exec.Command(python, path).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "sounding-lines") || strings.Contains(string(out), "PASSWORD") || strings.Contains(string(out), "container_details_partial") {
		t.Fatal("unexpected collector output")
	}
	failed := "import subprocess\ndef failed(*a,**k): raise FileNotFoundError()\nsubprocess.Popen=failed\n"
	os.WriteFile(path, []byte(failed+collector), 0600)
	out, err = exec.Command(python, path).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"docker_available":false`) || !strings.Contains(string(out), "docker_unavailable") {
		t.Fatal("missing partial status")
	}
}
