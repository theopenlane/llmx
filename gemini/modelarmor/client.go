package modelarmor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	armor "cloud.google.com/go/modelarmor/apiv1"
	"cloud.google.com/go/modelarmor/apiv1/modelarmorpb"
	"google.golang.org/api/option"

	"github.com/theopenlane/llmx"
)

const (
	// templateLocationIndex is where the location sits in projects/x/locations/y/templates/z
	templateLocationIndex = 3
	// templateNameParts is how many segments a full template resource name has
	templateNameParts = 6
	// modelarmorendpoint is the regional model armor api endpoint
	modelarmorendpoint = "modelarmor.%s.rep.googleapis.com:443"
)

var (
	// ErrTemplateInvalid is returned when the template is not a full resource name
	ErrTemplateInvalid = errors.New("modelarmor: template must be projects/<project>/locations/<location>/templates/<template>")
	// ErrSanitizeFailed is returned when the sanitize request itself failed
	ErrSanitizeFailed = errors.New("modelarmor: sanitize request failed")
	// ErrScreenIncomplete is returned when Model Armor ran only some filters or none, so the document is not screened
	ErrScreenIncomplete = errors.New("modelarmor: screening did not complete")
)

// Client screens documents against one Model Armor template
type Client struct {
	api      *armor.Client
	template string
}

// ensure the client satisfies the llmx screener seam
var _ llmx.Screener = (*Client)(nil)

// New builds a client for the template, calling its region with application default credentials
func New(ctx context.Context, template string) (*Client, error) {
	location, err := templateLocation(template)
	if err != nil {
		return nil, err
	}

	api, err := armor.NewClient(ctx, option.WithEndpoint(fmt.Sprintf(modelarmorendpoint, location)))
	if err != nil {
		return nil, err
	}

	return &Client{api: api, template: template}, nil
}

// NewScreener builds a client for the template when one is configured, an empty template leaves screening off
func NewScreener(ctx context.Context, template string) (llmx.Screener, error) {
	if template == "" {
		return nil, nil
	}

	// a failed build returns a nil interface rather than a typed nil client
	client, err := New(ctx, template)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// Template returns the template the client screens against
func (c *Client) Template() string {
	return c.template
}

// Close releases the underlying connection
func (c *Client) Close() error {
	return c.api.Close()
}

// Screen submits the pdf to Model Armor and reports which filters matched, skipped, or only need reporting
func (c *Client) Screen(ctx context.Context, pdf []byte) (llmx.Verdict, error) {
	resp, err := c.api.SanitizeUserPrompt(ctx, &modelarmorpb.SanitizeUserPromptRequest{
		Name: c.template,
		UserPromptData: &modelarmorpb.DataItem{
			DataItem: &modelarmorpb.DataItem_ByteItem{
				ByteItem: &modelarmorpb.ByteDataItem{ByteDataType: modelarmorpb.ByteDataItem_PDF, ByteData: pdf},
			},
		},
	})
	if err != nil {
		return llmx.Verdict{}, fmt.Errorf("%w: %w", ErrSanitizeFailed, err)
	}

	return verdictFrom(resp.GetSanitizationResult())
}

// templateLocation validates a template resource name and returns the region it lives in
func templateLocation(template string) (string, error) {
	parts := strings.Split(template, "/")
	if len(parts) != templateNameParts || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "templates" {
		return "", ErrTemplateInvalid
	}

	return parts[templateLocationIndex], nil
}

// verdictFrom reads a sanitization result, failing when Model Armor did not finish screening
func verdictFrom(result *modelarmorpb.SanitizationResult) (llmx.Verdict, error) {
	switch invocation := result.GetInvocationResult(); invocation {
	case modelarmorpb.InvocationResult_PARTIAL, modelarmorpb.InvocationResult_FAILURE:
		return llmx.Verdict{}, fmt.Errorf("%w: %s", ErrScreenIncomplete, invocation)
	}

	var verdict llmx.Verdict

	// the overall match state is set by any filter, so each filter's own result decides the outcome
	for name, filter := range result.GetFilterResults() {
		switch {
		case blockingMatch(filter):
			verdict.Blocked = true
			verdict.Filters = append(verdict.Filters, name)
		case blockingSkipped(filter):
			verdict.Skipped = append(verdict.Skipped, name)
		case sensitiveDataMatch(filter):
			verdict.Reported = append(verdict.Reported, name)
		}
	}

	slices.Sort(verdict.Filters)
	slices.Sort(verdict.Skipped)
	slices.Sort(verdict.Reported)

	return verdict, nil
}

// blockingMatch reports whether a filter other than sensitive data detection found something unsafe to send
func blockingMatch(filter *modelarmorpb.FilterResult) bool {
	return slices.Contains([]modelarmorpb.FilterMatchState{
		filter.GetPiAndJailbreakFilterResult().GetMatchState(),
		filter.GetRaiFilterResult().GetMatchState(),
		filter.GetMaliciousUriFilterResult().GetMatchState(),
		filter.GetCsamFilterFilterResult().GetMatchState(),
		filter.GetVirusScanFilterResult().GetMatchState(),
	}, modelarmorpb.FilterMatchState_MATCH_FOUND)
}

// blockingSkipped reports whether a filter that would block the document did not run
func blockingSkipped(filter *modelarmorpb.FilterResult) bool {
	return slices.Contains([]modelarmorpb.FilterExecutionState{
		filter.GetPiAndJailbreakFilterResult().GetExecutionState(),
		filter.GetRaiFilterResult().GetExecutionState(),
		filter.GetMaliciousUriFilterResult().GetExecutionState(),
		filter.GetCsamFilterFilterResult().GetExecutionState(),
		filter.GetVirusScanFilterResult().GetExecutionState(),
	}, modelarmorpb.FilterExecutionState_EXECUTION_SKIPPED)
}

// sensitiveDataMatch reports whether the filter found sensitive data, which reports legitimately contain
func sensitiveDataMatch(filter *modelarmorpb.FilterResult) bool {
	return slices.Contains([]modelarmorpb.FilterMatchState{
		filter.GetSdpFilterResult().GetInspectResult().GetMatchState(),
		filter.GetSdpFilterResult().GetDeidentifyResult().GetMatchState(),
	}, modelarmorpb.FilterMatchState_MATCH_FOUND)
}
