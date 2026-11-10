package script

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type Target struct {
	Interpreter string
	Path        string
	Module      string
	Jar         string
	Class       string
	Inline      bool
	Task        string
}

type syntax struct {
	valued      []string
	inline      []string
	module      string
	subcommands []string
	taskCommand string
	// bareTasks takes a name with no directory and no extension as a task.
	bareTasks bool
	builtins  []string
	// shortClusters reads -abc as -a -b -c, the last taking the rest as its
	// value.
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

// interpreter maps a binary name, in any case, to its interpreter, or "".
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

func Parse(exe string, args []string) (Target, bool) {
	var argv0 string

	if len(args) > 0 {
		argv0 = interpreter(filepath.Base(args[0]))
	}

	// bunx is a link to bun, so only argv[0] names it.
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

func isTaskName(a string) bool {
	return !strings.ContainsRune(a, '/') && filepath.Ext(a) == ""
}

// cluster reads -uc, -Bm or -Ilib the way getopt does: an option that takes
// a value takes the rest of the argument, else the next one.
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
