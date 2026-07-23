package github_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax"
	"github.com/gezacorp/metadatax/collectors/github"
)

type fakeProcessInfo struct {
	envs []string
	err  error
}

func (f *fakeProcessInfo) EnvironWithContext(ctx context.Context) ([]string, error) {
	return f.envs, f.err
}

// fakeTokenVerifier returns a single claim set for any token (claims), or, when
// byToken is set, claims keyed by the raw token so a test can model the runtime
// and id-token carrying different claims. A missing key verifies as an error.
type fakeTokenVerifier struct {
	claims  *github.RuntimeTokenClaims
	byToken map[string]*github.RuntimeTokenClaims
	err     error
}

func (f *fakeTokenVerifier) Verify(ctx context.Context, rawToken string) (*github.RuntimeTokenClaims, error) {
	if f.err != nil {
		return nil, f.err
	}

	if f.byToken != nil {
		claims, ok := f.byToken[rawToken]
		if !ok {
			return nil, errors.New("token not verifiable")
		}
		return claims, nil
	}

	return f.claims, nil
}

const sampleRuntimeToken = "eyJhbGciOiJSUzI1NiJ9.secret-token-value.sig"

func githubEnv() []string {
	return []string{
		"GITHUB_ACTIONS=true",
		"GITHUB_REPOSITORY=riptideslabs/daemon",
		"GITHUB_REPOSITORY_OWNER=riptideslabs",
		"GITHUB_REPOSITORY_ID=976787887",
		"GITHUB_REPOSITORY_OWNER_ID=202861820",
		"GITHUB_WORKFLOW=build",
		"GITHUB_JOB=build",
		"GITHUB_EVENT_NAME=push",
		"GITHUB_REF=refs/heads/aa",
		"GITHUB_REF_NAME=aa",
		"GITHUB_REF_TYPE=branch",
		"GITHUB_ACTOR=waynz0r",
		"GITHUB_ACTOR_ID=12624764",
		"GITHUB_RUN_ID=30012614543",
		"GITHUB_RUN_NUMBER=1806",
		"GITHUB_RUN_ATTEMPT=2",
		"GITHUB_SHA=aa5c5dd6adc9b38638e8b7251716d0515b6df0c7",
		"GITHUB_SERVER_URL=https://github.com",
		"GITHUB_API_URL=https://api.github.com",
		"INVOCATION_ID=92132f9ecaf94b63bed385f020aab39c",
		"ACTIONS_ORCHESTRATION_ID=3e77825e-0f10-4ef9-9852-a3291ffef79a.build.__default",
		"RUNNER_NAME=GitHub Actions 1000039223",
		"RUNNER_OS=Linux",
		"RUNNER_ARCH=X64",
		"RUNNER_ENVIRONMENT=github-hosted",
		"RUNNER_TRACKING_ID=github_1406e602-57f6-4769-b995-3c68b4b89af1",
		"ACTIONS_RUNTIME_TOKEN=" + sampleRuntimeToken,
	}
}

func matchingClaims() *github.RuntimeTokenClaims {
	return &github.RuntimeTokenClaims{
		Issuer:               github.GitHubActionsIssuer,
		ExpiresAt:            1784836604,
		RunID:                "30012614543",
		RunNumber:            "1806",
		RunType:              "full",
		SHA:                  "aa5c5dd6adc9b38638e8b7251716d0515b6df0c7",
		RepositoryID:         "976787887",
		RepositoryOwnerID:    "202861820",
		RepositoryVisibility: "internal",
		JobID:                "e679fd3b-5ebe-5859-aeb4-7920aa158817",
		RunnerID:             "1000039223",
		RunnerType:           "hosted",
		OrchestrationID:      "3e77825e-0f10-4ef9-9852-a3291ffef79a.build.__default",
		PlanID:               "3e77825e-0f10-4ef9-9852-a3291ffef79a",
		Scope:                "Actions.Results:a:b Actions.Runner:a:b",
		TrustTier:            "1",
	}
}

const sampleIDTokenRequestToken = "eyJhbGciOiJSUzI1NiJ9.id-token-request.sig"

// matchingIDClaims is a request-token claim set: the numeric core (as the
// runtime token) plus oidc_sub and an oidc_extra identity that agrees with
// githubEnv().
func matchingIDClaims() *github.RuntimeTokenClaims {
	c := matchingClaims()
	c.Subject = "repo:riptideslabs/daemon:ref:refs/heads/aa"
	c.Extra = `{"repository":"riptideslabs/daemon","repository_owner":"riptideslabs",` +
		`"repository_id":"976787887","repository_owner_id":"202861820",` +
		`"actor":"waynz0r","actor_id":"12624764","ref":"refs/heads/aa","ref_type":"branch",` +
		`"sha":"aa5c5dd6adc9b38638e8b7251716d0515b6df0c7","workflow":"build",` +
		`"workflow_ref":"riptideslabs/daemon/.github/workflows/go.yml@refs/heads/aa",` +
		`"job_workflow_ref":"riptideslabs/daemon/.github/workflows/go.yml@refs/heads/aa",` +
		`"run_id":"30012614543","run_number":"1806","run_attempt":"2",` +
		`"runner_environment":"github-hosted","event_name":"push"}`
	return c
}

func collect(t *testing.T, pid int32, opts ...github.CollectorOption) metadatax.MetadataContainer {
	t.Helper()

	c := github.New(opts...)
	ctx := metadatax.ContextWithPID(context.Background(), pid)
	md, err := c.GetMetadata(ctx)
	require.NoError(t, err)

	return md
}

func processInfoFunc(pi *fakeProcessInfo) github.ProcessInfoFunc {
	return func(ctx context.Context, pid int32) (github.ProcessInfo, error) {
		return pi, nil
	}
}

func TestGetMetadataReportedLabels(t *testing.T) {
	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: githubEnv()})),
		github.WithSkipTokenValidation(),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"true"}, labels["github:reported:actions"])
	assert.Equal(t, []string{"riptideslabs/daemon"}, labels["github:reported:repository"])
	assert.Equal(t, []string{"build"}, labels["github:reported:workflow"])
	assert.Equal(t, []string{"waynz0r"}, labels["github:reported:actor"])
	assert.Equal(t, []string{"30012614543"}, labels["github:reported:run-id"])
	assert.Equal(t, []string{"aa5c5dd6adc9b38638e8b7251716d0515b6df0c7"}, labels["github:reported:sha"])
	assert.Equal(t, []string{"Linux"}, labels["github:reported:runner:os"])
	assert.Equal(t, []string{"github-hosted"}, labels["github:reported:runner:environment"])
	assert.Equal(t, []string{"github_1406e602-57f6-4769-b995-3c68b4b89af1"}, labels["github:reported:runner:tracking-id"])
	assert.Equal(t, []string{"92132f9ecaf94b63bed385f020aab39c"}, labels["github:reported:invocation-id"])

	// token validation skipped -> not verified, no verified subtree.
	assert.Equal(t, []string{"false"}, labels["github:token-verified"])
	_, hasVerified := labels["github:verified:run-id"]
	assert.False(t, hasVerified)
}

func TestGetMetadataVerified(t *testing.T) {
	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: githubEnv()})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: matchingClaims()}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"true"}, labels["github:token-verified"])
	assert.Equal(t, []string{"30012614543"}, labels["github:verified:run-id"])
	assert.Equal(t, []string{"976787887"}, labels["github:verified:repository-id"])
	assert.Equal(t, []string{"internal"}, labels["github:verified:repository-visibility"])
	assert.Equal(t, []string{"e679fd3b-5ebe-5859-aeb4-7920aa158817"}, labels["github:verified:job-id"])
	assert.Equal(t, []string{"1"}, labels["github:verified:trust-tier"])
	assert.Equal(t, []string{github.GitHubActionsIssuer}, labels["github:verified:token-issuer"])
	assert.ElementsMatch(t, []string{"Actions.Results:a:b", "Actions.Runner:a:b"}, labels["github:verified:scope"])

	// reported subtree is still present alongside verified.
	assert.Equal(t, []string{"riptideslabs/daemon"}, labels["github:reported:repository"])
}

func TestGetMetadataTokenClaimMismatch(t *testing.T) {
	claims := matchingClaims()
	claims.RunID = "99999999999" // does not match GITHUB_RUN_ID

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: githubEnv()})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: claims}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"false"}, labels["github:token-verified"])
	_, hasVerified := labels["github:verified:run-id"]
	assert.False(t, hasVerified)
}

func TestGetMetadataVerifierError(t *testing.T) {
	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: githubEnv()})),
		github.WithTokenVerifier(&fakeTokenVerifier{err: errors.New("bad signature")}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"false"}, labels["github:token-verified"])
	_, hasVerified := labels["github:verified:run-id"]
	assert.False(t, hasVerified)
}

func TestGetMetadataIDTokenVerifiesHumanReadableIdentity(t *testing.T) {
	env := append(githubEnv(), "ACTIONS_ID_TOKEN_REQUEST_TOKEN="+sampleIDTokenRequestToken)

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: env})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: matchingIDClaims()}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"true"}, labels["github:token-verified"])
	// Name-bearing fields the runtime token can't attest are now verified.
	assert.Equal(t, []string{"riptideslabs/daemon"}, labels["github:verified:repository"])
	assert.Equal(t, []string{"riptideslabs"}, labels["github:verified:repository-owner"])
	assert.Equal(t, []string{"waynz0r"}, labels["github:verified:actor"])
	assert.Equal(t, []string{"refs/heads/aa"}, labels["github:verified:ref"])
	assert.Equal(t, []string{"build"}, labels["github:verified:workflow"])
	assert.Equal(t, []string{"repo:riptideslabs/daemon:ref:refs/heads/aa"}, labels["github:verified:subject"])
	// Numeric core still present too.
	assert.Equal(t, []string{"30012614543"}, labels["github:verified:run-id"])
}

func TestGetMetadataIDTokenIdentityMismatchFallsBackToRuntime(t *testing.T) {
	claims := matchingIDClaims()
	// oidc_extra claims a different repository than GITHUB_REPOSITORY.
	claims.Extra = `{"repository":"attacker/repo","repository_id":"976787887","actor":"mallory"}`

	env := append(githubEnv(), "ACTIONS_ID_TOKEN_REQUEST_TOKEN="+sampleIDTokenRequestToken)

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: env})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: claims}),
	)

	labels := md.GetLabels()
	// Runtime-token numeric verification still succeeds...
	assert.Equal(t, []string{"true"}, labels["github:token-verified"])
	assert.Equal(t, []string{"30012614543"}, labels["github:verified:run-id"])
	// ...but the mismatched identity is NOT emitted as verified.
	_, hasRepo := labels["github:verified:repository"]
	assert.False(t, hasRepo)
	_, hasActor := labels["github:verified:actor"]
	assert.False(t, hasActor)
}

func TestGetMetadataIDTokenMalformedExtraFallsBackToRuntime(t *testing.T) {
	// id-token verifies (signature + top-level cross-check) but its
	// oidc_extra is unparseable, so Identity() returns false and the
	// collector falls back to the runtime token's numeric claims. Uses a
	// per-token fake to give the two tokens genuinely different claim sets.
	idClaims := matchingClaims()
	idClaims.Extra = "{not valid json"

	env := append(githubEnv(), "ACTIONS_ID_TOKEN_REQUEST_TOKEN="+sampleIDTokenRequestToken)

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: env})),
		github.WithTokenVerifier(&fakeTokenVerifier{byToken: map[string]*github.RuntimeTokenClaims{
			sampleIDTokenRequestToken: idClaims,
			sampleRuntimeToken:        matchingClaims(),
		}}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"true"}, labels["github:token-verified"])
	assert.Equal(t, []string{"30012614543"}, labels["github:verified:run-id"])
	_, hasRepo := labels["github:verified:repository"]
	assert.False(t, hasRepo, "malformed oidc_extra must not yield verified identity")
}

func TestGetMetadataCrossCheckFailsClosedOnMissingEnvAnchor(t *testing.T) {
	// Valid signed token (fake verifier returns matching claims) but the
	// process blanks the anchor env vars to try to dodge the binding. Must
	// fail closed: token-verified=false, no verified subtree.
	env := []string{
		"GITHUB_ACTIONS=true",
		"GITHUB_REPOSITORY=attacker/repo",
		"ACTIONS_RUNTIME_TOKEN=" + sampleRuntimeToken,
		// GITHUB_RUN_ID / GITHUB_REPOSITORY_ID intentionally absent.
	}

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: env})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: matchingClaims()}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"false"}, labels["github:token-verified"])
	_, hasVerified := labels["github:verified:run-id"]
	assert.False(t, hasVerified)
}

func TestGetMetadataMissingToken(t *testing.T) {
	// GHA process, validation enabled, but no ACTIONS_RUNTIME_TOKEN present.
	env := []string{"GITHUB_ACTIONS=true", "GITHUB_RUN_ID=1", "GITHUB_REPOSITORY_ID=2"}

	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: env})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: matchingClaims()}),
	)

	labels := md.GetLabels()
	assert.Equal(t, []string{"false"}, labels["github:token-verified"])
	_, hasVerified := labels["github:verified:run-id"]
	assert.False(t, hasVerified)
}

func TestGetMetadataSoftError(t *testing.T) {
	failing := func(ctx context.Context, pid int32) (github.ProcessInfo, error) {
		return &fakeProcessInfo{err: errors.New("boom")}, nil
	}

	// Without WithSkipOnSoftError, the environ read failure surfaces as an error.
	c := github.New(github.CollectorWithProcessInfoFunc(failing), github.WithSkipTokenValidation())
	_, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 1234))
	assert.Error(t, err)

	// With it, the same failure yields empty metadata and no error.
	c = github.New(github.CollectorWithProcessInfoFunc(failing), github.WithSkipOnSoftError(), github.WithSkipTokenValidation())
	md, err := c.GetMetadata(metadatax.ContextWithPID(context.Background(), 1234))
	assert.NoError(t, err)
	assert.Empty(t, md.GetLabels())
}

func TestGetMetadataNotGitHubActions(t *testing.T) {
	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: []string{"HOME=/root", "PATH=/usr/bin"}})),
		github.WithSkipTokenValidation(),
	)

	assert.Empty(t, md.GetLabels())
}

func TestGetMetadataNoPID(t *testing.T) {
	c := github.New(github.WithSkipTokenValidation())
	_, err := c.GetMetadata(context.Background())
	assert.ErrorIs(t, err, metadatax.ErrPIDNotFound)
}

func TestGetMetadataNeverLeaksToken(t *testing.T) {
	md := collect(t, 1234,
		github.CollectorWithProcessInfoFunc(processInfoFunc(&fakeProcessInfo{envs: githubEnv()})),
		github.WithTokenVerifier(&fakeTokenVerifier{claims: matchingClaims()}),
	)

	for _, l := range md.GetLabelsSlice() {
		assert.NotContains(t, l.Value, sampleRuntimeToken, "label %s leaked the raw token", l.Name)
		assert.NotContains(t, l.Name, "ACTIONS_RUNTIME_TOKEN")
	}
}
