// Package github collects metadata for a process running as a GitHub Actions
// job. It emits two provenance-separated subtrees:
//
//   - reported:*  values taken verbatim from the process environment. Any
//     process can set these, so they are unproven even when token-verified is
//     true. The token carries only numeric IDs, so name-bearing fields
//     (repository name, actor) live here and are never attested.
//   - verified:*  claims from ACTIONS_RUNTIME_TOKEN, emitted only when its
//     signature verifies against GitHub's OIDC JWKS and its anchor claims match
//     the reported env (see crossCheck). token-verified=true gates this subtree.
//
// Consumers making trust decisions must rely on verified:* only.
package github

import (
	"context"
	"strings"

	"emperror.dev/errors"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/gezacorp/metadatax"
)

const name = "github"

// ProcessInfo is the subset of process introspection this collector needs:
// just the target process's environment. Kept as an interface so tests can
// inject a fake, mirroring the procfs collector's ProcessInfo pattern.
type ProcessInfo interface {
	EnvironWithContext(ctx context.Context) ([]string, error)
}

// ProcessInfoFunc resolves a ProcessInfo for a PID; overridable in tests.
type ProcessInfoFunc func(ctx context.Context, pid int32) (ProcessInfo, error)

type collector struct {
	processInfoFunc     ProcessInfoFunc
	tokenVerifier       TokenVerifier
	skipTokenValidation bool
	skipOnSoftError     bool

	mdContainerInitFunc func() metadatax.MetadataContainer
}

type CollectorOption func(*collector)

// CollectorWithProcessInfoFunc overrides how the target process is resolved.
func CollectorWithProcessInfoFunc(fn ProcessInfoFunc) CollectorOption {
	return func(c *collector) {
		c.processInfoFunc = fn
	}
}

// WithTokenVerifier overrides how ACTIONS_RUNTIME_TOKEN is verified. Use it to
// inject a fake in tests or point at a non-default OIDC issuer.
func WithTokenVerifier(verifier TokenVerifier) CollectorOption {
	return func(c *collector) {
		c.tokenVerifier = verifier
	}
}

// WithSkipTokenValidation disables JWT signature verification entirely; the
// collector then reports env-derived metadata only, with token-verified=false.
func WithSkipTokenValidation() CollectorOption {
	return func(c *collector) {
		c.skipTokenValidation = true
	}
}

// WithSkipOnSoftError makes process-lookup / environment-read failures return
// empty metadata instead of an error.
func WithSkipOnSoftError() CollectorOption {
	return func(c *collector) {
		c.skipOnSoftError = true
	}
}

// CollectorWithMetadataContainerInitFunc overrides how the root metadata
// container is constructed.
func CollectorWithMetadataContainerInitFunc(fn func() metadatax.MetadataContainer) CollectorOption {
	return func(c *collector) {
		c.mdContainerInitFunc = fn
	}
}

func New(opts ...CollectorOption) metadatax.Collector {
	c := &collector{}

	for _, f := range opts {
		f(c)
	}

	if c.processInfoFunc == nil {
		c.processInfoFunc = func(ctx context.Context, pid int32) (ProcessInfo, error) {
			return process.NewProcessWithContext(ctx, pid)
		}
	}

	if c.tokenVerifier == nil && !c.skipTokenValidation {
		c.tokenVerifier = NewOIDCTokenVerifier(GitHubActionsIssuer)
	}

	if c.mdContainerInitFunc == nil {
		c.mdContainerInitFunc = func() metadatax.MetadataContainer {
			return metadatax.New(metadatax.WithPrefix(name))
		}
	}

	return c
}

func (c *collector) GetMetadata(ctx context.Context) (metadatax.MetadataContainer, error) {
	md := c.mdContainerInitFunc()

	pid, found := metadatax.PIDFromContext(ctx)
	if !found {
		return nil, metadatax.ErrPIDNotFound
	}

	processInfo, err := c.processInfoFunc(ctx, pid)
	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not create new process instance", "pid", pid)
	}

	rawEnvs, err := processInfo.EnvironWithContext(ctx)
	if err != nil {
		if c.skipOnSoftError {
			return md, nil
		}

		return nil, errors.WrapIfWithDetails(err, "could not get process environment", "pid", pid)
	}

	env := parseEnv(rawEnvs)

	// Not a GitHub Actions process - report nothing, like the other
	// collectors' "not applicable" path.
	if env["GITHUB_ACTIONS"] != "true" {
		return md, nil
	}

	c.reported(env, md)
	c.verify(ctx, env, md)

	return md, nil
}

// reported emits the env-derived metadata. These values are self-reported by
// the process and unproven, so they live under the "reported" segment. Only a
// curated allowlist of keys is surfaced, which also guarantees no secret
// (e.g. ACTIONS_RUNTIME_TOKEN) is ever emitted.
func (c *collector) reported(env map[string]string, md metadatax.MetadataContainer) {
	rmd := md.Segment("reported")

	rmd.AddLabel("actions", env["GITHUB_ACTIONS"])
	rmd.AddLabel("repository", env["GITHUB_REPOSITORY"])
	rmd.AddLabel("repository-owner", env["GITHUB_REPOSITORY_OWNER"])
	rmd.AddLabel("repository-id", env["GITHUB_REPOSITORY_ID"])
	rmd.AddLabel("repository-owner-id", env["GITHUB_REPOSITORY_OWNER_ID"])
	rmd.AddLabel("workflow", env["GITHUB_WORKFLOW"])
	rmd.AddLabel("workflow-ref", env["GITHUB_WORKFLOW_REF"])
	rmd.AddLabel("job", env["GITHUB_JOB"])
	rmd.AddLabel("action", env["GITHUB_ACTION"])
	rmd.AddLabel("event-name", env["GITHUB_EVENT_NAME"])
	rmd.AddLabel("ref", env["GITHUB_REF"])
	rmd.AddLabel("ref-name", env["GITHUB_REF_NAME"])
	rmd.AddLabel("ref-type", env["GITHUB_REF_TYPE"])
	rmd.AddLabel("base-ref", env["GITHUB_BASE_REF"])
	rmd.AddLabel("head-ref", env["GITHUB_HEAD_REF"])
	rmd.AddLabel("actor", env["GITHUB_ACTOR"])
	rmd.AddLabel("actor-id", env["GITHUB_ACTOR_ID"])
	rmd.AddLabel("triggering-actor", env["GITHUB_TRIGGERING_ACTOR"])
	rmd.AddLabel("run-id", env["GITHUB_RUN_ID"])
	rmd.AddLabel("run-number", env["GITHUB_RUN_NUMBER"])
	rmd.AddLabel("run-attempt", env["GITHUB_RUN_ATTEMPT"])
	rmd.AddLabel("sha", env["GITHUB_SHA"])
	rmd.AddLabel("server-url", env["GITHUB_SERVER_URL"])
	rmd.AddLabel("api-url", env["GITHUB_API_URL"])
	rmd.AddLabel("invocation-id", env["INVOCATION_ID"])

	runnermd := rmd.Segment("runner")
	runnermd.AddLabel("name", env["RUNNER_NAME"])
	runnermd.AddLabel("os", env["RUNNER_OS"])
	runnermd.AddLabel("arch", env["RUNNER_ARCH"])
	runnermd.AddLabel("environment", env["RUNNER_ENVIRONMENT"])
	runnermd.AddLabel("tracking-id", env["RUNNER_TRACKING_ID"])
}

// verify validates the GitHub-signed tokens in the environment and, when a
// signature is valid and its claims cross-check against the reported env,
// emits the attested values under the "verified" segment with
// token-verified=true.
//
// Two tokens can carry identity: ACTIONS_RUNTIME_TOKEN (always present, but
// only numeric IDs) and ACTIONS_ID_TOKEN_REQUEST_TOKEN (present with
// `id-token: write`, and richer - its oidc_extra carries repository name,
// actor, ref, workflow, ...). The id-token is a superset, so when it verifies
// it is the single source for the verified subtree; otherwise the runtime
// token is used. This avoids emitting the overlapping numeric claims twice.
func (c *collector) verify(ctx context.Context, env map[string]string, md metadatax.MetadataContainer) {
	if c.skipTokenValidation || c.tokenVerifier == nil {
		md.AddLabel("token-verified", "false")
		return
	}

	if c.verifyIDToken(ctx, env, md) || c.verifyRuntimeToken(ctx, env, md) {
		md.AddLabel("token-verified", "true")
		return
	}

	md.AddLabel("token-verified", "false")
}

// verifyIDToken verifies ACTIONS_ID_TOKEN_REQUEST_TOKEN and, on success, emits
// both the numeric core claims and the human-readable identity from its
// signature-attested oidc_extra. Returns false when the token is absent,
// fails verification, or fails the cross-check.
func (c *collector) verifyIDToken(ctx context.Context, env map[string]string, md metadatax.MetadataContainer) bool {
	rawToken := env["ACTIONS_ID_TOKEN_REQUEST_TOKEN"]
	if rawToken == "" {
		return false
	}

	claims, err := c.tokenVerifier.Verify(ctx, rawToken)
	if err != nil || !crossCheck(claims, env) {
		return false
	}

	id, ok := claims.Identity()
	if !ok || !crossCheckIdentity(id, env) {
		// Signature verified but the identity payload is missing or doesn't
		// bind to this process: fall back to the runtime-token path rather
		// than emit unbound name fields.
		return false
	}

	vmd := md.Segment("verified")
	applyVerifiedClaims(vmd, claims)
	applyVerifiedIdentity(vmd, id, claims.Subject)

	return true
}

// verifyRuntimeToken verifies ACTIONS_RUNTIME_TOKEN and, on success, emits its
// numeric core claims. Returns false when the token is absent, fails
// verification, or fails the cross-check.
func (c *collector) verifyRuntimeToken(ctx context.Context, env map[string]string, md metadatax.MetadataContainer) bool {
	rawToken := env["ACTIONS_RUNTIME_TOKEN"]
	if rawToken == "" {
		return false
	}

	claims, err := c.tokenVerifier.Verify(ctx, rawToken)
	if err != nil || !crossCheck(claims, env) {
		return false
	}

	applyVerifiedClaims(md.Segment("verified"), claims)

	return true
}

func parseEnv(rawEnvs []string) map[string]string {
	env := make(map[string]string, len(rawEnvs))
	for _, e := range rawEnvs {
		key, value, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		env[key] = value
	}

	return env
}
