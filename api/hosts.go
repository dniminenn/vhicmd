package api

import (
	"encoding/json"
	"fmt"
)

type Host struct {
	HostName  string                 `json:"hypervisor_hostname"`
	Service   string                 `json:"service"`
	Zone      string                 `json:"zone"`
	Status    string                 `json:"status"`
	State     string                 `json:"state"`
	Updated   string                 `json:"updated_at"`
	Resources map[string]interface{} `json:"resources,omitempty"`
}

type HostListResponse struct {
	Hosts []Host `json:"hypervisors"`
}

func ListHosts(computeURL, token string) (HostListResponse, error) {
	var result HostListResponse

	url := fmt.Sprintf("%s/os-hypervisors/detail", computeURL)
	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to fetch hosts: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("hosts request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse hosts response: %v", err)
	}

	// Get detailed resource info for each host
	for i := range result.Hosts {
		details, err := GetHostDetails(computeURL, token, result.Hosts[i].HostName)
		if err == nil {
			result.Hosts[i].Resources = details.Resources
		}
	}

	return result, nil
}

func GetHostDetails(computeURL, token, hostName string) (Host, error) {
	var result Host

	url := fmt.Sprintf("%s/os-hypervisors/%s", computeURL, hostName)
	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to fetch host details: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("host details request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	var wrapper struct {
		Host Host `json:"hypervisor"`
	}
	err = json.Unmarshal([]byte(apiResp.Response), &wrapper)
	if err != nil {
		return result, fmt.Errorf("failed to parse host details: %v", err)
	}

	return wrapper.Host, nil
}
