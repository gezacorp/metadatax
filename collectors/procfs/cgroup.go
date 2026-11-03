package procfs

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gezacorp/metadatax"
)

// Kinds of the innermost systemd unit a process runs in.
const (
	KindService   = "service"
	KindApp       = "app"
	KindSnap      = "snap"
	KindContainer = "container"
	KindSession   = "session"
	KindTmux      = "tmux"
	KindTerminal  = "terminal"
	KindTransient = "transient"
	KindInit      = "init"
	KindScope     = "scope"
)

var (
	userManager    = regexp.MustCompile(`^user@\d+\.service$`)
	snapUnit       = regexp.MustCompile(`^snap\.([^.]+)\.(.+?)(?:-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})?\.(?:scope|service)$`)
	containerUnit  = regexp.MustCompile(`^(docker|cri-containerd|crio|libpod)-[0-9a-f]+\.scope$`)
	nspawnUnit     = regexp.MustCompile(`^systemd-nspawn@.+\.service$`)
	transientUnit  = regexp.MustCompile(`^run-(r[0-9a-f]+|u\d+|p\d+-i\d+|\d+)\.(?:service|scope)$`)
	dbusUnit       = regexp.MustCompile(`^dbus-:[\d.]+-(.+)@\d+\.service$`)
	appScopeRandom = regexp.MustCompile(`-[^-]+\.scope$`)
	appServiceRand = regexp.MustCompile(`@[^@]*\.service$`)
)

// CgroupPath is the path in a /proc/<pid>/cgroup file that places the
// process: the unified hierarchy's, else the named systemd hierarchy's on
// cgroup v1.
func CgroupPath(content string) string {
	var v1 string

	for line := range strings.Lines(content) {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}

		switch {
		case parts[0] == "0" && parts[1] == "":
			return parts[2]
		case parts[1] == "name=systemd":
			v1 = parts[2]
		}
	}

	return v1
}

// CgroupUnit is the systemd unit deepest in a cgroup path, under the system
// or a user's manager, and what its name says about what runs in it. The
// system manager reports a unit under a user's manager as the
// user@<uid>.service it is in; the path still names it.
type CgroupUnit struct {
	Unit string
	// Manager is "user" for a unit under a user@<uid>.service, else
	// "system".
	Manager string
	Kind    string
	// Name is the unit without what changes per instance: the template
	// instance of a service, the application ID of an app, <snap>.<app> of
	// a snap.
	Name     string
	Launcher string
	Snap     string
	SnapApp  string
	// ContainerUnit is the innermost container the path passes through,
	// also when the container runs a systemd of its own and the innermost
	// unit is one of its services; ContainerRuntime is what runs it.
	ContainerUnit    string
	ContainerRuntime string
}

// InnermostUnit reads the innermost unit of a cgroup path; ok is false when
// the path has neither a unit nor a container in it.
func InnermostUnit(path string) (CgroupUnit, bool) {
	var (
		u    CgroupUnit
		user bool
	)

	for elem := range strings.SplitSeq(path, "/") {
		if runtime := containerRuntime(elem); runtime != "" {
			u.ContainerUnit, u.ContainerRuntime = elem, runtime
		}

		if !strings.HasSuffix(elem, ".service") && !strings.HasSuffix(elem, ".scope") {
			continue
		}

		if u.Unit != "" && userManager.MatchString(u.Unit) {
			user = true
		}

		u.Unit = elem
	}

	if u.Unit == "" {
		if u.ContainerUnit == "" {
			return u, false
		}

		u.Unit = u.ContainerUnit
	}

	u.Manager = "system"

	if user {
		u.Manager = "user"
	}

	classify(&u)

	return u, true
}

// containerRuntime is what runs the container a cgroup path element is, or
// empty: a docker, containerd, CRI-O or podman scope, an LXD or Incus
// payload, a systemd-nspawn machine.
func containerRuntime(elem string) string {
	switch {
	case containerUnit.MatchString(elem):
		return containerUnit.FindStringSubmatch(elem)[1]
	case strings.HasPrefix(elem, "lxc.payload."):
		return "lxc"
	case nspawnUnit.MatchString(elem):
		return "nspawn"
	}

	return ""
}

func classify(u *CgroupUnit) {
	unit := u.Unit

	switch {
	case snapUnit.MatchString(unit):
		m := snapUnit.FindStringSubmatch(unit)
		u.Kind, u.Snap, u.SnapApp = KindSnap, m[1], m[2]
		u.Name = m[1] + "." + m[2]
	case unit == u.ContainerUnit:
		u.Kind = KindContainer
	case strings.HasPrefix(unit, "app-"):
		u.Kind = KindApp
		u.Launcher, u.Name = appID(unit)
	case strings.HasPrefix(unit, "tmux-spawn-") && strings.HasSuffix(unit, ".scope"):
		u.Kind = KindTmux
	case strings.HasPrefix(unit, "vte-spawn-") && strings.HasSuffix(unit, ".scope"):
		u.Kind = KindTerminal
	case transientUnit.MatchString(unit):
		u.Kind = KindTransient
	case dbusUnit.MatchString(unit):
		u.Kind, u.Name = KindService, unescape(dbusUnit.FindStringSubmatch(unit)[1])
	case strings.HasPrefix(unit, "session-") && strings.HasSuffix(unit, ".scope"):
		u.Kind = KindSession
	case unit == "init.scope":
		u.Kind = KindInit
	case strings.HasSuffix(unit, ".service"):
		u.Kind, u.Name = KindService, foldTemplate(unit)
	default:
		u.Kind, u.Name = KindScope, unit
	}
}

// appID reads the launcher and the application ID of a unit named the way
// desktop environments name the units they start apps in:
// app[-<launcher>]-<ApplicationID>-<RANDOM>.scope or
// app[-<launcher>]-<ApplicationID>[@<RANDOM>].service, dashes inside the
// parts escaped as \x2d.
func appID(unit string) (string, string) {
	s := strings.TrimPrefix(unit, "app-")

	switch {
	case strings.HasSuffix(s, ".scope"):
		s = appScopeRandom.ReplaceAllString(s, "")
	default:
		s = appServiceRand.ReplaceAllString(s, "")
		s = strings.TrimSuffix(s, ".service")
	}

	launcher, id, found := strings.Cut(s, "-")
	if !found {
		return "", unescape(s)
	}

	return unescape(launcher), unescape(id)
}

func foldTemplate(unit string) string {
	at := strings.IndexByte(unit, '@')
	if at < 0 {
		return unit
	}

	return unit[:at+1] + ".service"
}

// unescape decodes systemd's \xNN escapes. They are bytes, so a character
// beyond ASCII is several of them, decoded together into its UTF-8.
func unescape(s string) string {
	if !strings.Contains(s, `\x`) {
		return s
	}

	b := make([]byte, 0, len(s))

	for i := 0; i < len(s); i++ {
		if strings.HasPrefix(s[i:], `\x`) && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				b = append(b, byte(v))
				i += 3

				continue
			}
		}

		b = append(b, s[i])
	}

	return string(b)
}

func addCgroup(md metadatax.MetadataContainer, path string) {
	md.AddLabel("cgroup", path)

	u, ok := InnermostUnit(path)
	if !ok {
		return
	}

	cmd := md.Segment("cgroup")
	cmd.AddLabel("unit", u.Unit)
	cmd.AddLabel("manager", u.Manager)
	cmd.AddLabel("kind", u.Kind)

	for name, value := range map[string]string{
		"name":     u.Name,
		"launcher": u.Launcher,
		"snap":     u.Snap,
		"snap-app": u.SnapApp,
	} {
		if value != "" {
			cmd.AddLabel(name, value)
		}
	}

	if u.ContainerUnit != "" {
		cmd.Segment("container").
			AddLabel("unit", u.ContainerUnit).
			AddLabel("runtime", u.ContainerRuntime)
	}
}
