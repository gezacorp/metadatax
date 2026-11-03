package script

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Target is what an interpreter's command line runs.
type Target struct {
	Interpreter string
	// Path is the script file, as given on the command line.
	Path string
	// Module is a module or package run by name: python -m, java -m,
	// bun x.
	Module string
	Jar    string
	// Class is the main class java runs from its class path.
	Class string
	// Inline is set for code given on the command line itself.
	Inline bool
	// Task is a script of the project's package.json or deno.json run by
	// name: bun run dev, deno task start.
	Task string
}

type syntax struct {
	// valued are the options that take the next argument as their value.
	valued []string
	// inline are the options whose value is the code to run.
	inline []string
	// module is the option whose value is a module to run.
	module string
	// subcommands are skipped before the script, such as "run".
	subcommands []string
	// taskCommand is the subcommand whose argument is always a task.
	taskCommand string
	// bareTasks is set for interpreters taking a name with no directory
	// and no extension as a task, not a file.
	bareTasks bool
	// builtins are the commands that run no script of the user's.
	builtins []string
	// shortClusters is set for interpreters taking -abc as -a -b -c, the
	// last of which may take a value given as the rest of the argument.
	shortClusters bool
}

var syntaxes = map[string]syntax{
	"node": {
		valued: []string{"-r", "--require", "--import", "--loader", "--experimental-loader", "-C", "--conditions", "--title", "--env-file", "--input-type"},
		inline: []string{"-e", "--eval", "-p", "--print"},
	},
	"bun": {
		valued:      []string{"-r", "--preload", "--cwd", "--env-file", "--config", "-c"},
		inline:      []string{"-e", "--eval", "-p", "--print"},
		module:      "x",
		subcommands: []string{"run"},
		bareTasks:   true,
		builtins: []string{
			"test", "repl", "exec", "install", "i", "add", "a", "remove", "rm", "update", "link", "unlink",
			"pm", "build", "init", "create", "c", "upgrade", "outdated", "publish", "audit", "info", "patch",
		},
	},
	"deno": {
		valued:      []string{"-c", "--config", "--import-map", "--lock", "--cert", "--location", "--seed", "--env-file", "-L", "--log-level", "--cwd"},
		inline:      []string{"eval"},
		subcommands: []string{"run", "serve", "task", "test"},
		taskCommand: "task",
		builtins: []string{
			"lsp", "fmt", "lint", "check", "cache", "install", "uninstall", "upgrade", "compile", "repl", "doc",
			"info", "bench", "coverage", "init", "jupyter", "publish", "add", "remove", "outdated", "clean",
			"types", "vendor", "completions", "help",
		},
	},
	"python": {
		valued:        []string{"-W", "-X", "--check-hash-based-pycs"},
		inline:        []string{"-c"},
		module:        "-m",
		shortClusters: true,
	},
	"ruby": {
		valued:        []string{"-r", "-I", "-C", "-E", "-F", "-x", "--encoding"},
		inline:        []string{"-e"},
		shortClusters: true,
	},
	"perl": {
		valued:        []string{"-I", "-M", "-m", "-x", "-C"},
		inline:        []string{"-e", "-E"},
		shortClusters: true,
	},
	"java": {
		valued: []string{"-cp", "-classpath", "--class-path", "-p", "--module-path", "--add-modules", "--add-opens", "--add-exports", "-splash"},
		module: "-m",
	},
}

var (
	pythonName = regexp.MustCompile(`^python(\d+(\.\d+)*)?w?$`)
	perlName   = regexp.MustCompile(`^perl(\d+(\.\d+)*)?$`)
	rubyName   = regexp.MustCompile(`^ruby(\d+(\.\d+)*)?$`)
)

// interpreter is the interpreter a binary name is, or empty. Case is
// ignored: macOS framework builds run as Python.app/Contents/MacOS/Python.
func interpreter(base string) string {
	base = strings.ToLower(base)

	switch {
	case base == "node" || base == "nodejs":
		return "node"
	case base == "bun" || base == "deno" || base == "java":
		return base
	case base == "bunx":
		return "bunx"
	case pythonName.MatchString(base):
		return "python"
	case perlName.MatchString(base):
		return "perl"
	case rubyName.MatchString(base):
		return "ruby"
	}

	return ""
}

// Parse reads what the command line args of the process running exe runs;
// ok is false when the process is not an interpreter.
func Parse(exe string, args []string) (Target, bool) {
	var argv0 string

	if len(args) > 0 {
		argv0 = interpreter(filepath.Base(args[0]))
	}

	// bunx is a link to bun, which the exe resolves; only argv[0] tells the
	// two apart.
	in := argv0
	if in != "bunx" {
		in = interpreter(filepath.Base(exe))
	}

	if in == "" {
		in = argv0
	}

	if in == "" {
		return Target{}, false
	}

	t := Target{Interpreter: in}

	if in == "bunx" {
		t.Interpreter = "bun"

		if len(args) > 1 {
			t.Module = firstNonOption(args[1:])
		}

		return t, true
	}

	s := syntaxes[in]
	rest := args[1:]
	sub := ""

	for len(rest) > 0 {
		a := rest[0]
		rest = rest[1:]

		switch {
		case a == "--":
			if len(rest) > 0 {
				t.Path = rest[0]
			}

			return t, true
		case in == "java" && a == "-jar":
			if len(rest) > 0 {
				t.Jar = rest[0]
			}

			return t, true
		case s.module != "" && a == s.module:
			if len(rest) > 0 {
				t.Module = rest[0]
			}

			return t, true
		case slices.Contains(s.inline, a):
			t.Inline = true

			return t, true
		case slices.Contains(s.valued, a):
			if len(rest) > 0 {
				rest = rest[1:]
			}
		case strings.HasPrefix(a, "-"):
			if s.shortClusters {
				var done bool
				if rest, done = cluster(a, rest, s, &t); done {
					return t, true
				}
			}
		case sub == "" && slices.Contains(s.subcommands, a):
			sub = a
		case sub == "" && slices.Contains(s.builtins, a):
			return t, true
		default:
			switch {
			case in == "java":
				t.Class = a
			case sub != "" && sub == s.taskCommand, s.bareTasks && isTaskName(a):
				t.Task = a
			default:
				t.Path = a
			}

			return t, true
		}
	}

	return t, true
}

// isTaskName reports whether a names a task rather than a file: no
// directory and no extension, such as dev or start.
func isTaskName(a string) bool {
	return !strings.ContainsRune(a, '/') && filepath.Ext(a) == ""
}

// cluster reads a cluster of short options such as -uc, -Bm or -Ilib the
// way getopt does: an option that takes a value takes the rest of the
// argument, or the next one when nothing is left. It returns the arguments
// after the cluster, and whether the cluster ended the options with inline
// code or a module.
func cluster(a string, rest []string, s syntax, t *Target) ([]string, bool) {
	if strings.HasPrefix(a, "--") || len(a) < 2 {
		return rest, false
	}

	for i := 1; i < len(a); i++ {
		opt, value := "-"+string(a[i]), a[i+1:]

		switch {
		case slices.Contains(s.inline, opt):
			t.Inline = true

			return rest, true
		case opt == s.module:
			if value == "" && len(rest) > 0 {
				value = rest[0]
			}

			t.Module = value

			return rest, true
		case slices.Contains(s.valued, opt):
			if value == "" && len(rest) > 0 {
				rest = rest[1:]
			}

			return rest, false
		}
	}

	return rest, false
}

func firstNonOption(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}

	return ""
}
