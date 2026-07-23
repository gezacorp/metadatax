package github

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/gezacorp/metadatax"
)

// GitHubActionsIssuer is the OIDC issuer that signs ACTIONS_RUNTIME_TOKEN.
// It publishes a JWKS at /.well-known/jwks, making the token's RS256
// signature publicly verifiable.
const GitHubActionsIssuer = "https://token.actions.githubusercontent.com"

// RuntimeTokenClaims is the subset of ACTIONS_RUNTIME_TOKEN claims we surface
// and/or cross-check against the reported environment. All identity-bearing
// claims are numeric IDs rather than human-readable names.
type RuntimeTokenClaims struct {
	Issuer               string `json:"iss"`
	ExpiresAt            int64  `json:"exp"`
	RunID                string `json:"run_id"`
	RunNumber            string `json:"run_number"`
	RunType              string `json:"run_type"`
	SHA                  string `json:"sha"`
	RepositoryID         string `json:"repository_id"`
	RepositoryOwnerID    string `json:"repository_owner_id"`
	RepositoryVisibility string `json:"repository_visibility"`
	JobID                string `json:"job_id"`
	RunnerID             string `json:"runner_id"`
	RunnerType           string `json:"runner_type"`
	OrchestrationID      string `json:"orch_id"`
	PlanID               string `json:"plan_id"`
	Scope                string `json:"scp"`
	TrustTier            string `json:"trust_tier"`
	BillingOwnerID       string `json:"billing_owner_id"`
	OwnerID              string `json:"owner_id"`

	// Subject and Extra are populated on ACTIONS_ID_TOKEN_REQUEST_TOKEN (the
	// credential present when a job has `id-token: write`). That token is
	// signed by the same issuer, and its oidc_extra claim carries the
	// human-readable identity (repository name, actor, ref, workflow, ...)
	// that the runtime token only exposes as numeric IDs. Empty on the plain
	// runtime token.
	Subject string `json:"oidc_sub"`
	Extra   string `json:"oidc_extra"`
}

// IDTokenIdentity is the human-readable identity GitHub stages in the
// oidc_extra claim of ACTIONS_ID_TOKEN_REQUEST_TOKEN (and mints into the OIDC
// ID token). Because the enclosing token is signature-verified, these values
// are cryptographically attested, not self-reported.
// Fields are limited to what applyVerifiedIdentity emits or crossCheckIdentity
// binds; numeric operational claims (run_number, repository_visibility, ...)
// come from the token's top-level RuntimeTokenClaims instead, not oidc_extra.
type IDTokenIdentity struct {
	Repository        string `json:"repository"`
	RepositoryOwner   string `json:"repository_owner"`
	RepositoryID      string `json:"repository_id"`
	Actor             string `json:"actor"`
	ActorID           string `json:"actor_id"`
	Ref               string `json:"ref"`
	RefType           string `json:"ref_type"`
	RefProtected      string `json:"ref_protected"`
	BaseRef           string `json:"base_ref"`
	HeadRef           string `json:"head_ref"`
	SHA               string `json:"sha"`
	Workflow          string `json:"workflow"`
	WorkflowRef       string `json:"workflow_ref"`
	WorkflowSHA       string `json:"workflow_sha"`
	JobWorkflowRef    string `json:"job_workflow_ref"`
	JobWorkflowSHA    string `json:"job_workflow_sha"`
	EventName         string `json:"event_name"`
	RunID             string `json:"run_id"`
	RunAttempt        string `json:"run_attempt"`
	RunnerEnvironment string `json:"runner_environment"`
}

// Identity parses the oidc_extra claim, returning the attested identity and
// true when present. Absent (plain runtime token, or a future issuer that
// drops the claim) returns false, letting the collector degrade gracefully
// to the reported values rather than fail.
func (c *RuntimeTokenClaims) Identity() (*IDTokenIdentity, bool) {
	if c == nil || c.Extra == "" {
		return nil, false
	}

	id := &IDTokenIdentity{}
	if err := json.Unmarshal([]byte(c.Extra), id); err != nil {
		return nil, false
	}

	return id, true
}

// TokenVerifier verifies a raw ACTIONS_RUNTIME_TOKEN and returns its claims.
// An error means the token could not be cryptographically trusted (bad
// signature, wrong issuer, expired, or the verifier couldn't reach the JWKS).
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*RuntimeTokenClaims, error)
}

type oidcTokenVerifier struct {
	issuer string

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
}

// NewOIDCTokenVerifier returns a TokenVerifier that verifies tokens against
// the given OIDC issuer's published JWKS. The issuer's discovery document and
// keys are fetched lazily on first use and cached (go-oidc handles key
// rotation), so an offline host degrades to token-verified=false rather than
// failing collector construction.
func NewOIDCTokenVerifier(issuer string) TokenVerifier {
	return &oidcTokenVerifier{issuer: issuer}
}

func (v *oidcTokenVerifier) Verify(ctx context.Context, rawToken string) (*RuntimeTokenClaims, error) {
	verifier, err := v.idTokenVerifier(ctx)
	if err != nil {
		return nil, err
	}

	// SkipClientIDCheck: the runtime token's audience is dynamic
	// ("vso:<guid>"), not a fixed client ID, so audience is not a useful
	// gate here. Signature, issuer, and expiry are still enforced.
	idToken, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, errors.WrapIf(err, "could not verify token")
	}

	claims := &RuntimeTokenClaims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, errors.WrapIf(err, "could not parse token claims")
	}

	return claims, nil
}

func (v *oidcTokenVerifier) idTokenVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.verifier != nil {
		return v.verifier, nil
	}

	provider, err := oidc.NewProvider(ctx, v.issuer)
	if err != nil {
		return nil, errors.WrapIf(err, "could not initialize oidc provider")
	}

	v.verifier = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})

	return v.verifier, nil
}

// crossCheck binds the cryptographically verified token to this process's
// reported run. It fails closed: a valid signature alone is not enough — the
// token's identifying claims must actually match the env the process presents.
//
// Anchor claims (run id + repository id) MUST be present in both the token and
// the env and be equal. A real ACTIONS_RUNTIME_TOKEN always carries them, so
// requiring them rejects a process that blanks its env to dodge the binding
// while presenting a valid-but-unrelated token. For the remaining overlapping
// claims, when the token carries the claim the env must match it (a blank or
// differing env value fails).
func crossCheck(claims *RuntimeTokenClaims, env map[string]string) bool {
	if claims == nil {
		return false
	}

	anchors := []struct {
		claim  string
		envKey string
	}{
		{claims.RunID, "GITHUB_RUN_ID"},
		{claims.RepositoryID, "GITHUB_REPOSITORY_ID"},
	}

	for _, a := range anchors {
		if a.claim == "" || a.claim != env[a.envKey] {
			return false
		}
	}

	others := []struct {
		claim  string
		envKey string
	}{
		{claims.RunNumber, "GITHUB_RUN_NUMBER"},
		{claims.SHA, "GITHUB_SHA"},
		{claims.RepositoryOwnerID, "GITHUB_REPOSITORY_OWNER_ID"},
		{claims.OrchestrationID, "ACTIONS_ORCHESTRATION_ID"},
	}

	for _, o := range others {
		if o.claim != "" && o.claim != env[o.envKey] {
			return false
		}
	}

	return true
}

func applyVerifiedClaims(vmd metadatax.MetadataContainer, claims *RuntimeTokenClaims) {
	vmd.AddLabel("run-id", claims.RunID)
	vmd.AddLabel("run-number", claims.RunNumber)
	vmd.AddLabel("run-type", claims.RunType)
	vmd.AddLabel("sha", claims.SHA)
	vmd.AddLabel("repository-id", claims.RepositoryID)
	vmd.AddLabel("repository-owner-id", claims.RepositoryOwnerID)
	vmd.AddLabel("repository-visibility", claims.RepositoryVisibility)
	vmd.AddLabel("job-id", claims.JobID)
	vmd.AddLabel("runner-id", claims.RunnerID)
	vmd.AddLabel("runner-type", claims.RunnerType)
	vmd.AddLabel("orch-id", claims.OrchestrationID)
	vmd.AddLabel("plan-id", claims.PlanID)
	vmd.AddLabel("trust-tier", claims.TrustTier)
	vmd.AddLabel("token-issuer", claims.Issuer)

	if claims.ExpiresAt > 0 {
		vmd.AddLabel("token-expires-at", time.Unix(claims.ExpiresAt, 0).UTC().Format(time.RFC3339))
	}

	for scope := range strings.FieldsSeq(claims.Scope) {
		vmd.AddLabel("scope", scope)
	}
}

// crossCheckIdentity binds the id-token's human-readable identity to the
// reported env, fail-closed on the repository name + id anchors (both must be
// present and equal). Other overlapping fields must match when present.
func crossCheckIdentity(id *IDTokenIdentity, env map[string]string) bool {
	if id == nil {
		return false
	}

	if id.Repository == "" || id.Repository != env["GITHUB_REPOSITORY"] {
		return false
	}
	if id.RepositoryID == "" || id.RepositoryID != env["GITHUB_REPOSITORY_ID"] {
		return false
	}

	others := []struct {
		value  string
		envKey string
	}{
		{id.Actor, "GITHUB_ACTOR"},
		{id.Ref, "GITHUB_REF"},
		{id.SHA, "GITHUB_SHA"},
		{id.RepositoryOwner, "GITHUB_REPOSITORY_OWNER"},
		{id.RunID, "GITHUB_RUN_ID"},
	}

	for _, o := range others {
		if o.value != "" && o.value != env[o.envKey] {
			return false
		}
	}

	return true
}

// applyVerifiedIdentity emits the human-readable identity fields the runtime
// token cannot attest. These come from a signature-verified id-token, so they
// belong in the verified subtree.
func applyVerifiedIdentity(vmd metadatax.MetadataContainer, id *IDTokenIdentity, subject string) {
	vmd.AddLabel("subject", subject)
	vmd.AddLabel("repository", id.Repository)
	vmd.AddLabel("repository-owner", id.RepositoryOwner)
	vmd.AddLabel("actor", id.Actor)
	vmd.AddLabel("actor-id", id.ActorID)
	vmd.AddLabel("ref", id.Ref)
	vmd.AddLabel("ref-type", id.RefType)
	vmd.AddLabel("ref-protected", id.RefProtected)
	vmd.AddLabel("base-ref", id.BaseRef)
	vmd.AddLabel("head-ref", id.HeadRef)
	vmd.AddLabel("workflow", id.Workflow)
	vmd.AddLabel("workflow-ref", id.WorkflowRef)
	vmd.AddLabel("workflow-sha", id.WorkflowSHA)
	vmd.AddLabel("job-workflow-ref", id.JobWorkflowRef)
	vmd.AddLabel("job-workflow-sha", id.JobWorkflowSHA)
	vmd.AddLabel("event-name", id.EventName)
	vmd.AddLabel("run-attempt", id.RunAttempt)
	vmd.AddLabel("runner-environment", id.RunnerEnvironment)
}
