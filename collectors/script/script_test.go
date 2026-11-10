package script_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/script"
)

type processInfo struct {
	exe  string
	args []string
	cwd  string
}

func (p *processInfo) ExeWithContext(context.Context) (string, error) {
	return p.exe, nil
}

func (p *processInfo) CmdlineSliceWithContext(context.Context) ([]string, error) {
	return p.args, nil
}

func (p *processInfo) CwdWithContext(context.Context) (string, error) {
	return p.cwd, nil
}

func write(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// collect runs the collector for p, with root standing for the process's
// root directory.
func collect(t *testing.T, root string, p *processInfo) metadatax.Labels {
	t.Helper()

	md, err := script.New(
		script.CollectorWithProcessInfoFunc(func(context.Context, int32) (script.ProcessInfo, error) {
			return p, nil
		}),
		script.CollectorWithRootFunc(func(int32) string { return root }),
	).GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
	require.NoError(t, err)

	return md.GetLabels()
}

func TestCollector_NodeScriptInAnNPMPackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pkg := "/home/u/.npm/_npx/1/node_modules/@agentclientprotocol/claude-agent-acp"
	write(t, filepath.Join(root, pkg, "package.json"), `{"name": "@agentclientprotocol/claude-agent-acp", "version": "0.5.1"}`)
	write(t, filepath.Join(root, pkg, "dist/package.json"), `{"type": "module"}`)

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/node",
		args: []string{"node", pkg + "/dist/index.js"},
	})

	assert.Equal(t, metadatax.Labels{
		"script:interpreter":     {"node"},
		"script:path":            {pkg + "/dist/index.js"},
		"script:package:manager": {"npm"},
		"script:package:name":    {"@agentclientprotocol/claude-agent-acp"},
		"script:package:version": {"0.5.1"},
		"script:package:path":    {pkg},
	}, labels)
}

func TestCollector_RelativeScriptResolvesAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, "/srv/app/package.json"), `{"name": "app"}`)

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/node",
		args: []string{"node", "./src/../server.js"},
		cwd:  "/srv/app",
	})

	assert.Equal(t, []string{"/srv/app/server.js"}, labels["script:path"])
	assert.Equal(t, []string{"app"}, labels["script:package:name"])
	assert.NotContains(t, labels, "script:package:version")
}

func TestCollector_ScriptWithoutPackage(t *testing.T) {
	t.Parallel()

	labels := collect(t, t.TempDir(), &processInfo{
		exe:  "/usr/bin/python3",
		args: []string{"python3", "/opt/tools/agent.py"},
	})

	assert.Equal(t, metadatax.Labels{
		"script:interpreter": {"python"},
		"script:path":        {"/opt/tools/agent.py"},
	}, labels)
}

func TestCollector_PythonDistribution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	site := "/usr/lib/python3/dist-packages"
	write(t, filepath.Join(root, site, "httpie/__main__.py"), "")
	write(t, filepath.Join(root, site, "httpie-3.2.2.dist-info/METADATA"), "Metadata-Version: 2.1\nName: httpie\nVersion: 3.2.2\n\nlong description\n")
	write(t, filepath.Join(root, site, "httpie-3.2.2.dist-info/RECORD"), "httpie/__init__.py,sha256=x,1\nhttpie/__main__.py,sha256=y,2\n")
	write(t, filepath.Join(root, site, "requests-2.31.0.dist-info/METADATA"), "Name: requests\nVersion: 2.31.0\n")
	write(t, filepath.Join(root, site, "requests-2.31.0.dist-info/RECORD"), "requests/__init__.py,sha256=z,3\n")

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/python3.12",
		args: []string{"python3", site + "/httpie/__main__.py"},
	})

	assert.Equal(t, []string{"pypi"}, labels["script:package:manager"])
	assert.Equal(t, []string{"httpie"}, labels["script:package:name"])
	assert.Equal(t, []string{"3.2.2"}, labels["script:package:version"])
	assert.Equal(t, []string{site}, labels["script:package:path"])
}

func TestCollector_PythonNamespacePackage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	site := "/srv/venv/lib/python3.13/site-packages"
	write(t, filepath.Join(root, site, "google/cloud/storage/blob.py"), "")
	write(t, filepath.Join(root, site, "google_api_core-2.24.0.dist-info/METADATA"), "Name: google-api-core\nVersion: 2.24.0\n")
	write(t, filepath.Join(root, site, "google_api_core-2.24.0.dist-info/RECORD"), "google/api_core/__init__.py,sha256=a,1\n")
	write(t, filepath.Join(root, site, "google_cloud_storage-3.1.0.dist-info/METADATA"), "Name: google-cloud-storage\nVersion: 3.1.0\n")
	write(t, filepath.Join(root, site, "google_cloud_storage-3.1.0.dist-info/RECORD"), "google/cloud/storage/__init__.py,sha256=b,1\ngoogle/cloud/storage/blob.py,sha256=c,2\n")

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/python3",
		args: []string{"python3", site + "/google/cloud/storage/blob.py"},
	})

	assert.Equal(t, []string{"google-cloud-storage"}, labels["script:package:name"])
	assert.Equal(t, []string{"3.1.0"}, labels["script:package:version"])
}

func link(t *testing.T, target, path string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.Symlink(target, path))
}

func TestCollector_NPMGlobalCLIThroughItsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pkg := "/opt/homebrew/lib/node_modules/@linkedapi/linkedin-cli"
	write(t, filepath.Join(root, pkg, "package.json"), `{"name": "@linkedapi/linkedin-cli", "version": "1.2.0"}`)
	write(t, filepath.Join(root, pkg, "bin/run.js"), "#!/usr/bin/env node\n")
	link(t, "../lib/node_modules/@linkedapi/linkedin-cli/bin/run.js", filepath.Join(root, "/opt/homebrew/bin/linkedin"))

	labels := collect(t, root, &processInfo{
		exe:  "/opt/homebrew/Cellar/node/26.10.0_1/bin/node",
		args: []string{"node", "/opt/homebrew/bin/linkedin", "whoami"},
	})

	assert.Equal(t, []string{pkg + "/bin/run.js"}, labels["script:path"])
	assert.Equal(t, []string{"@linkedapi/linkedin-cli"}, labels["script:package:name"])
	assert.Equal(t, []string{"1.2.0"}, labels["script:package:version"])
}

func TestCollector_AbsoluteSymlinkResolvesInsideTheRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, "/app/package.json"), `{"name": "app"}`)
	write(t, filepath.Join(root, "/app/server.js"), "")
	link(t, "/app/server.js", filepath.Join(root, "/usr/local/bin/server"))

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/node",
		args: []string{"node", "/usr/local/bin/server"},
	})

	assert.Equal(t, []string{"/app/server.js"}, labels["script:path"])
	assert.Equal(t, []string{"app"}, labels["script:package:name"])
}

func pythonTool(t *testing.T, root, env, script, dist, version, module string) {
	t.Helper()

	site := filepath.Join(root, env, "lib/python3.13/site-packages")
	info := filepath.Join(site, dist+"-"+version+".dist-info")
	write(t, filepath.Join(info, "METADATA"), "Metadata-Version: 2.4\nName: "+dist+"\nVersion: "+version+"\n")
	write(t, filepath.Join(info, "entry_points.txt"), "[console_scripts]\n"+script+" = "+module+":main\n\n[other]\nx = y\n")
	write(t, filepath.Join(info, "RECORD"), "../../../bin/"+script+",sha256=x,1\n")
	write(t, filepath.Join(root, env, "bin", script), "#!"+env+"/bin/python\nfrom "+module+" import main\n")
}

func TestCollector_PythonConsoleScript(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := "/home/u/.local/share/uv/tools/google-agents-cli"
	pythonTool(t, root, env, "agents-cli", "google-agents-cli", "0.3.1", "google.agents.cli.main")
	pythonTool(t, root, env, "other-tool", "other", "1.0", "other.main")
	link(t, env+"/bin/agents-cli", filepath.Join(root, "/home/u/.local/bin/agents-cli"))

	labels := collect(t, root, &processInfo{
		exe:  "/home/u/.local/share/uv/python/cpython-3.13.5/bin/python3.13",
		args: []string{env + "/bin/python", "/home/u/.local/bin/agents-cli", "run"},
	})

	assert.Equal(t, []string{env + "/bin/agents-cli"}, labels["script:path"])
	assert.Equal(t, []string{"pypi"}, labels["script:package:manager"])
	assert.Equal(t, []string{"google-agents-cli"}, labels["script:package:name"])
	assert.Equal(t, []string{"0.3.1"}, labels["script:package:version"])
	assert.Equal(t, []string{env + "/lib/python3.13/site-packages"}, labels["script:package:path"])
}

func TestCollector_PythonConsoleScriptFoundThroughTheInterpreter(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	env := "/opt/venvs/llm"
	pythonTool(t, root, env, "llm", "llm", "0.19", "llm.cli")
	write(t, filepath.Join(root, "/usr/local/bin/llm"), "#!/opt/venvs/llm/bin/python\nfrom llm.cli import main\n")

	labels := collect(t, root, &processInfo{
		exe:  "/usr/bin/python3.13",
		args: []string{env + "/bin/python", "/usr/local/bin/llm", "chat"},
	})

	assert.Equal(t, []string{"llm"}, labels["script:package:name"])
	assert.Equal(t, []string{"0.19"}, labels["script:package:version"])
}

func TestCollector_ModuleAndInline(t *testing.T) {
	t.Parallel()

	assert.Equal(t, metadatax.Labels{
		"script:interpreter": {"python"},
		"script:module":      {"http.server"},
	}, collect(t, t.TempDir(), &processInfo{exe: "/usr/bin/python3", args: []string{"python3", "-m", "http.server"}}))

	assert.Equal(t, metadatax.Labels{
		"script:interpreter": {"node"},
		"script:inline":      {"true"},
	}, collect(t, t.TempDir(), &processInfo{exe: "/usr/bin/node", args: []string{"node", "-e", "1"}}))
}

func TestCollector_TaskBelongsToTheProjectInTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, "/srv/web/package.json"), `{"name": "web", "version": "2.0.0", "scripts": {"dev": "vite"}}`)

	labels := collect(t, root, &processInfo{
		exe:  "/home/u/.bun/bin/bun",
		args: []string{"bun", "run", "dev"},
		cwd:  "/srv/web/src",
	})

	assert.Equal(t, metadatax.Labels{
		"script:interpreter":     {"bun"},
		"script:task":            {"dev"},
		"script:package:manager": {"npm"},
		"script:package:name":    {"web"},
		"script:package:version": {"2.0.0"},
		"script:package:path":    {"/srv/web"},
	}, labels)
}

func setTime(t *testing.T, path string, d time.Duration) {
	t.Helper()

	ts := time.Now().Add(d)
	require.NoError(t, os.Chtimes(path, ts, ts))
}

func TestCollector_PythonAnswerIsCachedUntilSitePackagesChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	site := "/usr/lib/python3/dist-packages"
	write(t, filepath.Join(root, site, "httpie/__main__.py"), "")
	write(t, filepath.Join(root, site, "httpie-3.2.2.dist-info/METADATA"), "Name: httpie\nVersion: 3.2.2\n")
	write(t, filepath.Join(root, site, "httpie-3.2.2.dist-info/RECORD"), "httpie/__main__.py,sha256=y,2\n")
	setTime(t, filepath.Join(root, site), -time.Hour)

	c := newCollector(root, &processInfo{
		exe:  "/usr/bin/python3.12",
		args: []string{"python3", site + "/httpie/__main__.py"},
	})

	assert.Equal(t, []string{"httpie"}, labelsOf(t, c)["script:package:name"])

	require.NoError(t, os.Remove(filepath.Join(root, site, "httpie-3.2.2.dist-info/RECORD")))
	assert.Equal(t, []string{"httpie"}, labelsOf(t, c)["script:package:name"], "cached while site-packages is unchanged")

	require.NoError(t, os.MkdirAll(filepath.Join(root, site, "requests-2.31.0.dist-info"), 0o755))
	setTime(t, filepath.Join(root, site), time.Minute)
	assert.NotContains(t, labelsOf(t, c), "script:package:name", "looked up again once a distribution was installed")
}

func TestCollector_NPMAnswerIsCachedUntilPackageJSONChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest := filepath.Join(root, "/srv/app/package.json")
	write(t, manifest, `{"name": "app", "version": "1.0.0"}`)
	setTime(t, manifest, -time.Hour)

	c := newCollector(root, &processInfo{
		exe:  "/usr/bin/node",
		args: []string{"node", "/srv/app/server.js"},
	})

	assert.Equal(t, []string{"1.0.0"}, labelsOf(t, c)["script:package:version"])

	write(t, manifest, `{"name": "app", "version": "2.0.0"}`)
	setTime(t, manifest, time.Minute)
	assert.Equal(t, []string{"2.0.0"}, labelsOf(t, c)["script:package:version"])
}

func newCollector(root string, p *processInfo) metadatax.Collector {
	return script.New(
		script.CollectorWithProcessInfoFunc(func(context.Context, int32) (script.ProcessInfo, error) {
			return p, nil
		}),
		script.CollectorWithRootFunc(func(int32) string { return root }),
	)
}

func labelsOf(t *testing.T, c metadatax.Collector) metadatax.Labels {
	t.Helper()

	md, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 4242))
	require.NoError(t, err)

	return md.GetLabels()
}

func TestCollector_JavaJar(t *testing.T) {
	t.Parallel()

	labels := collect(t, t.TempDir(), &processInfo{
		exe:  "/usr/bin/java",
		args: []string{"java", "-jar", "app.jar"},
		cwd:  "/opt/app",
	})

	assert.Equal(t, metadatax.Labels{
		"script:interpreter": {"java"},
		"script:jar":         {"/opt/app/app.jar"},
	}, labels)
}

func TestCollector_NotAnInterpreter(t *testing.T) {
	t.Parallel()

	assert.Empty(t, collect(t, t.TempDir(), &processInfo{exe: "/usr/bin/curl", args: []string{"curl", "x"}}))
}

func TestCollector_Self(t *testing.T) {
	t.Parallel()

	md, err := script.New().GetMetadata(metadatax.ContextWithPID(context.Background(), int32(os.Getpid())))
	require.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}
