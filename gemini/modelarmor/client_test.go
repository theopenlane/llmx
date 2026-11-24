package modelarmor

import (
	"testing"

	"cloud.google.com/go/modelarmor/apiv1/modelarmorpb"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestTemplateLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		template         string
		expectedLocation string
		expectedErr      error
	}{
		{name: "full resource name", template: "projects/p/locations/us-central1/templates/t", expectedLocation: "us-central1"},
		{name: "empty", template: "", expectedErr: ErrTemplateInvalid},
		{name: "template id only", template: "t", expectedErr: ErrTemplateInvalid},
		{name: "wrong segment names", template: "projects/p/regions/us-central1/templates/t", expectedErr: ErrTemplateInvalid},
		{name: "extra segment", template: "projects/p/locations/us-central1/templates/t/versions/1", expectedErr: ErrTemplateInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location, err := templateLocation(tt.template)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)

				return
			}

			assert.NilError(t, err)
			assert.Check(t, location == tt.expectedLocation)
		})
	}
}

func TestVerdictFrom(t *testing.T) {
	result := &modelarmorpb.SanitizationResult{
		InvocationResult: modelarmorpb.InvocationResult_SUCCESS,
		FilterResults: map[string]*modelarmorpb.FilterResult{
			"pi_and_jailbreak": jailbreakResult(modelarmorpb.FilterMatchState_MATCH_FOUND, modelarmorpb.FilterExecutionState_EXECUTION_SUCCESS),
			"malicious_uris":   uriResult(modelarmorpb.FilterExecutionState_EXECUTION_SKIPPED),
			"sdp":              sdpResult(modelarmorpb.FilterMatchState_MATCH_FOUND),
			"rai":              raiResult(modelarmorpb.FilterMatchState_NO_MATCH_FOUND),
		},
	}

	verdict, err := verdictFrom(result)

	assert.NilError(t, err)
	assert.Check(t, verdict.Blocked)
	assert.Check(t, cmp.DeepEqual(verdict.Filters, []string{"pi_and_jailbreak"}))
	assert.Check(t, cmp.DeepEqual(verdict.Skipped, []string{"malicious_uris"}))
	assert.Check(t, cmp.DeepEqual(verdict.Reported, []string{"sdp"}))
}

func TestVerdictFromClean(t *testing.T) {
	verdict, err := verdictFrom(&modelarmorpb.SanitizationResult{
		FilterResults: map[string]*modelarmorpb.FilterResult{
			"rai": raiResult(modelarmorpb.FilterMatchState_NO_MATCH_FOUND),
		},
	})

	assert.NilError(t, err)
	assert.Check(t, !verdict.Blocked)
	assert.Check(t, len(verdict.Filters) == 0 && len(verdict.Skipped) == 0 && len(verdict.Reported) == 0)
}

func TestVerdictFromIncomplete(t *testing.T) {
	t.Parallel()

	for _, invocation := range []modelarmorpb.InvocationResult{modelarmorpb.InvocationResult_PARTIAL, modelarmorpb.InvocationResult_FAILURE} {
		t.Run(invocation.String(), func(t *testing.T) {
			t.Parallel()

			_, err := verdictFrom(&modelarmorpb.SanitizationResult{InvocationResult: invocation})

			assert.ErrorIs(t, err, ErrScreenIncomplete)
		})
	}
}

func jailbreakResult(match modelarmorpb.FilterMatchState, execution modelarmorpb.FilterExecutionState) *modelarmorpb.FilterResult {
	return &modelarmorpb.FilterResult{FilterResult: &modelarmorpb.FilterResult_PiAndJailbreakFilterResult{
		PiAndJailbreakFilterResult: &modelarmorpb.PiAndJailbreakFilterResult{MatchState: match, ExecutionState: execution},
	}}
}

func uriResult(execution modelarmorpb.FilterExecutionState) *modelarmorpb.FilterResult {
	return &modelarmorpb.FilterResult{FilterResult: &modelarmorpb.FilterResult_MaliciousUriFilterResult{
		MaliciousUriFilterResult: &modelarmorpb.MaliciousUriFilterResult{ExecutionState: execution},
	}}
}

func raiResult(match modelarmorpb.FilterMatchState) *modelarmorpb.FilterResult {
	return &modelarmorpb.FilterResult{FilterResult: &modelarmorpb.FilterResult_RaiFilterResult{
		RaiFilterResult: &modelarmorpb.RaiFilterResult{MatchState: match, ExecutionState: modelarmorpb.FilterExecutionState_EXECUTION_SUCCESS},
	}}
}

func sdpResult(match modelarmorpb.FilterMatchState) *modelarmorpb.FilterResult {
	return &modelarmorpb.FilterResult{FilterResult: &modelarmorpb.FilterResult_SdpFilterResult{
		SdpFilterResult: &modelarmorpb.SdpFilterResult{Result: &modelarmorpb.SdpFilterResult_InspectResult{
			InspectResult: &modelarmorpb.SdpInspectResult{MatchState: match},
		}},
	}}
}
