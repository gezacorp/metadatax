package script

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Package is the package a script belongs to.
type Package struct {
	Manager string
	Name    string
	Version string
	// Path is the package's directory as the process sees it.
	Path string
}

const maxManifestSize = 1 << 20

var sitePackages = regexp.MustCompile(`/(site|dist)-packages/`)

// findPackage finds the package file belongs to, reading files under root
// and adding to d what the answer depends on; argv0 is the interpreter as
// the process was started with it.
func findPackage(root, interpreter, file, argv0 string, d deps) (Package, bool) {
	switch interpreter {
	case "node", "bun", "deno":
		return npmPackage(root, file, d)
	case "python":
		if pkg, ok := pythonPackage(root, file, d); ok {
			return pkg, true
		}

		return pythonConsoleScript(root, file, argv0, d)
	}

	return Package{}, false
}

// npmPackage is the nearest package.json above file that names a package;
// those without a name only set options for the files under them.
func npmPackage(root, file string, d deps) (Package, bool) {
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		manifest := filepath.Join(dir, "package.json")
		d.add(root, dir)
		d.add(root, manifest)

		b, err := readSmall(filepath.Join(root, manifest))
		if err == nil {
			var m struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}

			if json.Unmarshal(b, &m) == nil && m.Name != "" {
				return Package{Manager: "npm", Name: m.Name, Version: m.Version, Path: dir}, true
			}
		}

		if dir == "/" || dir == "." {
			return Package{}, false
		}
	}
}

// pythonPackage is the distribution installed in a site-packages directory
// whose dist-info RECORD lists file itself. Matching the file, not its
// top-level directory, keeps namespace packages apart: google-cloud-storage
// and google-api-core both install under google/.
func pythonPackage(root, file string, d deps) (Package, bool) {
	loc := sitePackages.FindStringIndex(file)
	if loc == nil {
		return Package{}, false
	}

	site := file[:loc[1]-1]
	rel := file[loc[1]:]
	d.add(root, site)

	infos, err := filepath.Glob(filepath.Join(root, site, "*.dist-info"))
	if err != nil {
		return Package{}, false
	}

	for _, info := range infos {
		if !recordHas(filepath.Join(info, "RECORD"), rel) {
			continue
		}

		name, version := distMetadata(filepath.Join(info, "METADATA"))
		if name == "" {
			continue
		}

		return Package{Manager: "pypi", Name: name, Version: version, Path: site}, true
	}

	return Package{}, false
}

// pythonConsoleScript is the distribution that declares file as one of its
// console scripts: the launcher pip, pipx or uv writes into an
// environment's bin directory, which imports the package's code from that
// environment's site-packages. The environment is the one the launcher is
// in, or the one of the interpreter it was started with, <env>/bin/python.
func pythonConsoleScript(root, file, argv0 string, d deps) (Package, bool) {
	envs := []string{filepath.Dir(filepath.Dir(file))}

	if filepath.IsAbs(argv0) {
		if env := filepath.Dir(filepath.Dir(argv0)); env != envs[0] {
			envs = append(envs, env)
		}
	}

	script := filepath.Base(file)

	for _, env := range envs {
		d.add(root, filepath.Join(env, "lib"))

		for _, pattern := range []string{"lib/python3*/site-packages", "lib/python3*/dist-packages"} {
			sites, _ := filepath.Glob(filepath.Join(root, env, pattern))

			for _, site := range sites {
				d.add(root, strings.TrimPrefix(site, root))

				infos, _ := filepath.Glob(filepath.Join(site, "*.dist-info"))

				for _, info := range infos {
					if !entryPointsHave(filepath.Join(info, "entry_points.txt"), script) {
						continue
					}

					name, version := distMetadata(filepath.Join(info, "METADATA"))
					if name == "" {
						continue
					}

					return Package{Manager: "pypi", Name: name, Version: version, Path: strings.TrimPrefix(site, root)}, true
				}
			}
		}
	}

	return Package{}, false
}

// entryPointsHave reports whether an entry_points.txt declares script as a
// console or GUI script.
func entryPointsHave(path, script string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scripts := false

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		if strings.HasPrefix(line, "[") {
			scripts = line == "[console_scripts]" || line == "[gui_scripts]"

			continue
		}

		if name, _, ok := strings.Cut(line, "="); scripts && ok && strings.TrimSpace(name) == script {
			return true
		}
	}

	return false
}

// recordHas reports whether a dist-info RECORD lists rel, a path relative
// to site-packages.
func recordHas(record, rel string) bool {
	f, err := os.Open(record)
	if err != nil {
		return false
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if p, _, _ := strings.Cut(s.Text(), ","); p == rel {
			return true
		}
	}

	return false
}

func distMetadata(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	var name, version string

	s := bufio.NewScanner(f)
	for s.Scan() && (name == "" || version == "") {
		line := s.Text()
		if line == "" {
			break
		}

		if v, ok := strings.CutPrefix(line, "Name: "); ok {
			name = v
		}

		if v, ok := strings.CutPrefix(line, "Version: "); ok {
			version = v
		}
	}

	return name, version
}

// readSmall reads a file of at most maxManifestSize bytes.
func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := io.ReadAll(io.LimitReader(f, maxManifestSize+1))
	if err != nil {
		return nil, err
	}

	if len(b) > maxManifestSize {
		return nil, os.ErrInvalid
	}

	return b, nil
}
