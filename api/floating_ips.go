package api

import (
	"encoding/json"
	"fmt"
)

// FloatingIP represents a neutron floating IP.
type FloatingIP struct {
	ID                string `json:"id"`
	FloatingIPAddress string `json:"floating_ip_address"`
	FloatingNetworkID string `json:"floating_network_id"`
	FixedIPAddress    string `json:"fixed_ip_address"`
	PortID            string `json:"port_id"`
	RouterID          string `json:"router_id"`
	Status            string `json:"status"`
	Description       string `json:"description"`
	ProjectID         string `json:"project_id,omitempty"`
}

// FloatingIPListResponse represents the response for listing floating IPs.
type FloatingIPListResponse struct {
	FloatingIPs []FloatingIP `json:"floatingips"`
}

// ListFloatingIPs fetches the floating IPs matching the query, every page.
func ListFloatingIPs(networkURL, token string, queryParams map[string]string) (FloatingIPListResponse, error) {
	result := FloatingIPListResponse{FloatingIPs: []FloatingIP{}}

	listURL := fmt.Sprintf("%s/v2.0/floatingips", networkURL)
	err := listPages(listURL, token, queryParams, "floatingips_links", func(body []byte) error {
		var page FloatingIPListResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("failed to parse floating IPs response: %v", err)
		}
		result.FloatingIPs = append(result.FloatingIPs, page.FloatingIPs...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("failed to list floating IPs: %v", err)
	}
	return result, nil
}

// CreateFloatingIP allocates a floating IP from an external network.
func CreateFloatingIP(networkURL, token, externalNetworkID string) (FloatingIP, error) {
	var wrapper struct {
		FloatingIP FloatingIP `json:"floatingip"`
	}

	request := map[string]map[string]string{"floatingip": {"floating_network_id": externalNetworkID}}

	apiResp, err := callPOST(fmt.Sprintf("%s/v2.0/floatingips", networkURL), token, request)
	if err != nil {
		return wrapper.FloatingIP, fmt.Errorf("failed to create floating IP: %v", err)
	}
	if apiResp.ResponseCode != 201 {
		return wrapper.FloatingIP, fmt.Errorf("create floating IP request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.FloatingIP, fmt.Errorf("failed to parse create floating IP response: %v", err)
	}
	return wrapper.FloatingIP, nil
}

// SetFloatingIPPort associates a floating IP with a port, or disassociates it when portID is empty.
func SetFloatingIPPort(networkURL, token, floatingIPID, portID string) (FloatingIP, error) {
	var wrapper struct {
		FloatingIP FloatingIP `json:"floatingip"`
	}

	// port_id must be sent as null to disassociate, so it is never left out.
	var request struct {
		FloatingIP struct {
			PortID *string `json:"port_id"`
		} `json:"floatingip"`
	}
	if portID != "" {
		request.FloatingIP.PortID = &portID
	}

	apiResp, err := callPUT(fmt.Sprintf("%s/v2.0/floatingips/%s", networkURL, floatingIPID), token, request)
	if err != nil {
		return wrapper.FloatingIP, fmt.Errorf("failed to update floating IP: %v", err)
	}
	if apiResp.ResponseCode != 200 {
		return wrapper.FloatingIP, fmt.Errorf("update floating IP request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.FloatingIP, fmt.Errorf("failed to parse update floating IP response: %v", err)
	}
	return wrapper.FloatingIP, nil
}

// DeleteFloatingIP releases a floating IP by ID.
func DeleteFloatingIP(networkURL, token, floatingIPID string) error {
	apiResp, err := callDELETE(fmt.Sprintf("%s/v2.0/floatingips/%s", networkURL, floatingIPID), token)
	if err != nil {
		return fmt.Errorf("failed to delete floating IP: %v", err)
	}
	if apiResp.ResponseCode != 204 {
		return fmt.Errorf("delete floating IP request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}
	return nil
}

// GetFloatingIPIDByAddress returns a UUID as is, or the ID of the one floating IP with that address.
func GetFloatingIPIDByAddress(networkURL, token, address string) (string, error) {
	if isUuid(address) {
		return address, nil
	}

	resp, err := ListFloatingIPs(networkURL, token, map[string]string{"floating_ip_address": address})
	if err != nil {
		return "", err
	}

	var found []FloatingIP
	for _, fip := range resp.FloatingIPs {
		if fip.FloatingIPAddress == address {
			found = append(found, fip)
		}
	}

	if len(found) == 0 {
		return "", fmt.Errorf("no floating IP found with address %s", address)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple floating IPs found with address %s", address)
	}
	return found[0].ID, nil
}
