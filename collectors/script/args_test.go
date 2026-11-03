package script_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gezacorp/metadatax/collectors/script"
)

func TestParse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		exe  string
		args []string
		want script.Target
	}{
		{
			name: "node script with a preload",
			exe:  "/opt/homebrew/Cellar/node/26.10.0_1/bin/node",
			args: []string{"/opt/homebrew/bin/node", "--require", "./trace.js", "--enable-source-maps", "dist/index.js", "--port", "3000"},
			want: script.Target{
				Interpreter: "node",
				Path:        "dist/index.js",
			},
		},
		{
			name: "node eval",
			exe:  "/usr/bin/node",
			args: []string{"node", "-e", "console.log(1)"},
			want: script.Target{
				Interpreter: "node",
				Inline:      true,
			},
		},
		{
			name: "nodejs name",
			exe:  "/usr/bin/nodejs",
			args: []string{"nodejs", "server.js"},
			want: script.Target{
				Interpreter: "node",
				Path:        "server.js",
			},
		},
		{
			name: "bun run",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "run", "--cwd", "app", "src/main.ts"},
			want: script.Target{
				Interpreter: "bun",
				Path:        "src/main.ts",
			},
		},
		{
			name: "bun run task",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "run", "dev", "--port", "3000"},
			want: script.Target{
				Interpreter: "bun",
				Task:        "dev",
			},
		},
		{
			name: "bun task without run",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "dev"},
			want: script.Target{
				Interpreter: "bun",
				Task:        "dev",
			},
		},
		{
			name: "bun built-in command",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "build", "./index.ts", "--outdir", "dist"},
			want: script.Target{
				Interpreter: "bun",
			},
		},
		{
			name: "bun run extensionless file in a directory",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "run", "./server"},
			want: script.Target{
				Interpreter: "bun",
				Path:        "./server",
			},
		},
		{
			name: "bunx through its symlink to bun",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bunx", "@modelcontextprotocol/server-filesystem", "/tmp"},
			want: script.Target{
				Interpreter: "bun",
				Module:      "@modelcontextprotocol/server-filesystem",
			},
		},
		{
			name: "deno language server",
			exe:  "/opt/homebrew/Cellar/deno/2.5.6/bin/deno",
			args: []string{"deno", "lsp"},
			want: script.Target{
				Interpreter: "deno",
			},
		},
		{
			name: "deno compile",
			exe:  "/usr/local/bin/deno",
			args: []string{"deno", "compile", "--output", "app", "main.ts"},
			want: script.Target{
				Interpreter: "deno",
			},
		},
		{
			name: "deno task",
			exe:  "/usr/local/bin/deno",
			args: []string{"deno", "task", "--cwd", "app", "start"},
			want: script.Target{
				Interpreter: "deno",
				Task:        "start",
			},
		},
		{
			name: "bun x",
			exe:  "/home/u/.bun/bin/bun",
			args: []string{"bun", "x", "@modelcontextprotocol/server-filesystem", "/tmp"},
			want: script.Target{
				Interpreter: "bun",
				Module:      "@modelcontextprotocol/server-filesystem",
			},
		},
		{
			name: "bunx",
			exe:  "/home/u/.bun/bin/bunx",
			args: []string{"bunx", "--bun", "cowsay", "hi"},
			want: script.Target{
				Interpreter: "bun",
				Module:      "cowsay",
			},
		},
		{
			name: "deno run",
			exe:  "/usr/local/bin/deno",
			args: []string{"deno", "run", "-A", "--config", "deno.json", "main.ts"},
			want: script.Target{
				Interpreter: "deno",
				Path:        "main.ts",
			},
		},
		{
			name: "python script",
			exe:  "/usr/bin/python3.12",
			args: []string{"/usr/bin/python3", "-u", "-W", "ignore", "agent.py", "--model", "x"},
			want: script.Target{
				Interpreter: "python",
				Path:        "agent.py",
			},
		},
		{
			name: "python module",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-m", "http.server", "8000"},
			want: script.Target{
				Interpreter: "python",
				Module:      "http.server",
			},
		},
		{
			name: "python attached module",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-mhttp.server"},
			want: script.Target{
				Interpreter: "python",
				Module:      "http.server",
			},
		},
		{
			name: "python module ending a cluster",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-Bm", "http.server", "8000"},
			want: script.Target{
				Interpreter: "python",
				Module:      "http.server",
			},
		},
		{
			name: "python module attached inside a cluster",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-Imhttp.server"},
			want: script.Target{
				Interpreter: "python",
				Module:      "http.server",
			},
		},
		{
			name: "python valued option ending a cluster",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-uW", "ignore", "agent.py"},
			want: script.Target{
				Interpreter: "python",
				Path:        "agent.py",
			},
		},
		{
			name: "python inline in a cluster",
			exe:  "/usr/bin/python3",
			args: []string{"python3", "-uc", "print(1)"},
			want: script.Target{
				Interpreter: "python",
				Inline:      true,
			},
		},
		{
			name: "python console script wrapper",
			exe:  "/usr/bin/python3.12",
			args: []string{"/usr/bin/python3", "/usr/local/bin/httpie", "GET", "example.com"},
			want: script.Target{
				Interpreter: "python",
				Path:        "/usr/local/bin/httpie",
			},
		},
		{
			name: "ruby with include dir",
			exe:  "/usr/bin/ruby",
			args: []string{"ruby", "-I", "lib", "bin/server"},
			want: script.Target{
				Interpreter: "ruby",
				Path:        "bin/server",
			},
		},
		{
			name: "perl inline",
			exe:  "/usr/bin/perl",
			args: []string{"perl", "-ne", "print"},
			want: script.Target{
				Interpreter: "perl",
				Inline:      true,
			},
		},
		{
			name: "java jar",
			exe:  "/usr/lib/jvm/java-21/bin/java",
			args: []string{"java", "-Xmx1g", "-Dfoo=bar", "-jar", "app.jar", "--server.port=8080"},
			want: script.Target{
				Interpreter: "java",
				Jar:         "app.jar",
			},
		},
		{
			name: "java main class",
			exe:  "/usr/bin/java",
			args: []string{"java", "-cp", "lib/*", "com.acme.Main"},
			want: script.Target{
				Interpreter: "java",
				Class:       "com.acme.Main",
			},
		},
		{
			name: "macOS framework python",
			exe:  "/Library/Frameworks/Python.framework/Versions/3.13/Resources/Python.app/Contents/MacOS/Python",
			args: []string{"/Library/Frameworks/Python.framework/Versions/3.13/Resources/Python.app/Contents/MacOS/Python", "agent.py"},
			want: script.Target{
				Interpreter: "python",
				Path:        "agent.py",
			},
		},
		{
			name: "interpreter known only by argv",
			exe:  "",
			args: []string{"python3", "app.py"},
			want: script.Target{
				Interpreter: "python",
				Path:        "app.py",
			},
		},
		{
			name: "interpreter without a program",
			exe:  "/usr/bin/python3",
			args: []string{"python3"},
			want: script.Target{
				Interpreter: "python",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := script.Parse(tc.exe, tc.args)
			assert.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParse_NotAnInterpreter(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"curl", "-s", "https://example.com"},
		{"pythonista", "x.py"},
		{"/usr/bin/node-gyp", "build"},
	} {
		_, ok := script.Parse("", args)
		assert.False(t, ok, args[0])
	}
}
