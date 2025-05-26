package api

import (
	"encoding/json"
	"fmt"
)

type LimitsResponse struct {
	Limits map[string]interface{} `json:"limits"`
}

func GetLimits(computeURL, token string, includeReserved bool, tenantID string) (LimitsResponse, error) {
	var result LimitsResponse

	url := fmt.Sprintf("%s/limits", computeURL)
	if includeReserved {
		url += "?reserved=1"
	}
	if tenantID != "" {
		if includeReserved {
			url += "&"
		} else {
			url += "?"
		}
		url += fmt.Sprintf("tenant_id=%s", tenantID)
	}

	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to fetch limits: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("limits request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse limits response: %v", err)
	}

	return result, nil
}

func GetAbsoluteLimits(limits map[string]interface{}) map[string]interface{} {
	if absolute, ok := limits["absolute"].(map[string]interface{}); ok {
		return absolute
	}
	return nil
}
