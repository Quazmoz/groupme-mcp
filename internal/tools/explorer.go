package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Quazmoz/groupme-mcp/internal/client"
)

func parseEndpointProbeCandidates(args map[string]interface{}) ([]client.EndpointProbeCandidate, error) {
	if rawCandidates, ok := args["candidates"].([]interface{}); ok && len(rawCandidates) > 0 {
		payload, err := json.Marshal(rawCandidates)
		if err != nil {
			return nil, fmt.Errorf("failed to encode candidates: %w", err)
		}

		var candidates []client.EndpointProbeCandidate
		if err := json.Unmarshal(payload, &candidates); err != nil {
			return nil, fmt.Errorf("failed to decode candidates: %w", err)
		}
		return candidates, nil
	}

	if candidatesJSON := getString(args, "candidates_json"); candidatesJSON != "" {
		var candidates []client.EndpointProbeCandidate
		if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil {
			return nil, fmt.Errorf("candidates_json must be a JSON array of {endpoint, method?, label?}: %w", err)
		}
		return candidates, nil
	}

	endpoint := getString(args, "endpoint")
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint or candidates_json is required")
	}

	return []client.EndpointProbeCandidate{
		{
			Label:    getString(args, "label"),
			Method:   getString(args, "method"),
			Endpoint: endpoint,
		},
	}, nil
}

func probeAPIEndpointsHandler(ctx context.Context, c *client.Client, args map[string]interface{}) (interface{}, error) {
	candidates, err := parseEndpointProbeCandidates(args)
	if err != nil {
		return nil, err
	}

	return c.ProbeEndpoints(ctx, candidates, client.EndpointProbeOptions{
		StopOnFirstSuccess: getBoolArg(args, "stop_on_first_success", false),
		IncludeHeaders:     getBoolArg(args, "include_headers", false),
		MaxBodyBytes:       getInt(args, "max_body_bytes", client.DefaultProbeMaxBodyBytes),
	})
}
