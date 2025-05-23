package api

import (
	"encoding/json"
	"fmt"
	"time"
)

type TenantUsage struct {
	Start              string        `json:"start"`
	Stop               string        `json:"stop"`
	TenantID           string        `json:"tenant_id"`
	TotalHours         float64       `json:"total_hours"`
	TotalLocalGBUsage  float64       `json:"total_local_gb_usage"`
	TotalMemoryMBUsage float64       `json:"total_memory_mb_usage"`
	TotalVCPUsUsage    float64       `json:"total_vcpus_usage"`
	ServerUsages       []ServerUsage `json:"server_usages,omitempty"`
}

type ServerUsage struct {
	EndedAt    *string `json:"ended_at"`
	Flavor     string  `json:"flavor"`
	Hours      float64 `json:"hours"`
	InstanceID string  `json:"instance_id"`
	LocalGB    int     `json:"local_gb"`
	MemoryMB   int     `json:"memory_mb"`
	Name       string  `json:"name"`
	StartedAt  string  `json:"started_at"`
	State      string  `json:"state"`
	TenantID   string  `json:"tenant_id"`
	Uptime     int     `json:"uptime"`
	VCPUs      int     `json:"vcpus"`
}

type TenantUsageResponse struct {
	TenantUsages []TenantUsage `json:"tenant_usages"`
}

type SingleTenantUsageResponse struct {
	TenantUsage TenantUsage `json:"tenant_usage"`
}

func GetTenantUsage(computeURL, token string, start, end time.Time, detailed bool) (TenantUsageResponse, error) {
	var result TenantUsageResponse

	url := fmt.Sprintf("%s/os-simple-tenant-usage", computeURL)
	if !start.IsZero() {
		url += fmt.Sprintf("?start=%s", start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		if start.IsZero() {
			url += "?"
		} else {
			url += "&"
		}
		url += fmt.Sprintf("end=%s", end.Format(time.RFC3339))
	}
	if detailed {
		if start.IsZero() && end.IsZero() {
			url += "?"
		} else {
			url += "&"
		}
		url += "detailed=1"
	}

	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to fetch tenant usage: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("tenant usage request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse tenant usage response: %v", err)
	}

	return result, nil
}

func GetSingleTenantUsage(computeURL, token, tenantID string, start, end time.Time) (SingleTenantUsageResponse, error) {
	var result SingleTenantUsageResponse

	url := fmt.Sprintf("%s/os-simple-tenant-usage/%s", computeURL, tenantID)
	if !start.IsZero() {
		url += fmt.Sprintf("?start=%s", start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		if start.IsZero() {
			url += "?"
		} else {
			url += "&"
		}
		url += fmt.Sprintf("end=%s", end.Format(time.RFC3339))
	}

	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to fetch tenant usage: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("tenant usage request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse tenant usage response: %v", err)
	}

	return result, nil
}
