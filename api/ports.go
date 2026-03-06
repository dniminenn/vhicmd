package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AllowedAddressPair represents an allowed address pair on a port.
// When port security is enabled, this authorizes the port to send/receive
// traffic from additional IP addresses beyond its fixed IPs (e.g. Keepalived VIPs).
type AllowedAddressPair struct {
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address,omitempty"`
}

// Port represents a Neutron port
type Port struct {
	AdminStateUp        bool                 `json:"admin_state_up,omitempty"`
	AllowedAddressPairs []AllowedAddressPair `json:"allowed_address_pairs,omitempty"`
	BindingHostID       string               `json:"binding:host_id,omitempty"`
	BindingVnicType     string               `json:"binding:vnic_type,omitempty"`
	CreatedAt           string               `json:"created_at,omitempty"`
	DeviceID            string               `json:"device_id,omitempty"`
	DeviceOwner         string               `json:"device_owner,omitempty"`
	DNSDomain           string               `json:"dns_domain,omitempty"`
	DNSName             string               `json:"dns_name,omitempty"`
	FixedIPs            []IPInfo             `json:"fixed_ips,omitempty"`
	ID                  string               `json:"id,omitempty"`
	MACAddress          string               `json:"mac_address,omitempty"`
	Name                string               `json:"name,omitempty"`
	NetworkID           string               `json:"network_id"`
	PortSecurityEnabled *bool                `json:"port_security_enabled,omitempty"`
	SecurityGroups      []string             `json:"security_groups,omitempty"`
	Status              string               `json:"status,omitempty"`
	UpdatedAt           string               `json:"updated_at,omitempty"`
}

// PortCreateRequest represents the request body for port creation
type PortCreateRequest struct {
	Port Port `json:"port"`
}

// PortCreateResponse represents the response from port creation
type PortCreateResponse struct {
	Port Port `json:"port"`
}

// PortListResponse represents the response for listing ports
type PortListResponse struct {
	Ports []Port `json:"ports"`
}

// CreatePort creates a new port with specified parameters
func CreatePort(baseURL, token string, networkID, macAddress, name string, fixedIPs []IPInfo, allowedPairs []AllowedAddressPair) (PortCreateResponse, error) {
	var result PortCreateResponse

	url := fmt.Sprintf("%s/v2.0/ports", baseURL)

	request := PortCreateRequest{
		Port: Port{
			NetworkID:           networkID,
			MACAddress:          macAddress,
			Name:                name,
			FixedIPs:            fixedIPs,
			AllowedAddressPairs: allowedPairs,
		},
	}

	apiResp, err := callPOST(url, token, request)
	if err != nil {
		return result, fmt.Errorf("failed to create port: %v", err)
	}

	if apiResp.ResponseCode != 201 {
		return result, fmt.Errorf("create port request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse create port response: %v", err)
	}

	return result, nil
}

// PortUpdateRequest represents the request body for updating a port
type PortUpdateRequest struct {
	Port PortUpdateFields `json:"port"`
}

// PortUpdateFields contains only the fields that can be updated on a port
type PortUpdateFields struct {
	AllowedAddressPairs []AllowedAddressPair `json:"allowed_address_pairs,omitempty"`
	PortSecurityEnabled *bool                `json:"port_security_enabled,omitempty"`
	SecurityGroups      *[]string            `json:"security_groups,omitempty"`
}

// UpdatePortAllowedAddressPairs sets the allowed address pairs on an existing port
func UpdatePortAllowedAddressPairs(baseURL, token, portID string, pairs []AllowedAddressPair) (Port, error) {
	return UpdatePort(baseURL, token, portID, PortUpdateFields{
		AllowedAddressPairs: pairs,
	})
}

// UpdatePort updates a port with the given fields
func UpdatePort(baseURL, token, portID string, fields PortUpdateFields) (Port, error) {
	var wrapper struct {
		Port Port `json:"port"`
	}

	url := fmt.Sprintf("%s/v2.0/ports/%s", baseURL, portID)

	request := PortUpdateRequest{
		Port: fields,
	}

	apiResp, err := callPUT(url, token, request)
	if err != nil {
		return wrapper.Port, fmt.Errorf("failed to update port: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return wrapper.Port, fmt.Errorf("update port request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &wrapper)
	if err != nil {
		return wrapper.Port, fmt.Errorf("failed to parse update port response: %v", err)
	}

	return wrapper.Port, nil
}

// ListPorts fetches list of ports with optional query parameters
func ListPorts(baseURL, token string, queryParams map[string]string) (PortListResponse, error) {
	var result PortListResponse

	url := fmt.Sprintf("%s/v2.0/ports", baseURL)
	if len(queryParams) > 0 {
		url += "?"
		for key, value := range queryParams {
			url += fmt.Sprintf("%s=%s&", key, value)
		}
		url = strings.TrimSuffix(url, "&") // Remove trailing &
	}

	apiResp, err := callGET(url, token)
	if err != nil {
		return result, fmt.Errorf("failed to list ports: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return result, fmt.Errorf("list ports request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &result)
	if err != nil {
		return result, fmt.Errorf("failed to parse list ports response: %v", err)
	}

	return result, nil
}

// GetPortDetails fetches details of a specific port by ID
func GetPortDetails(baseURL, token, portID string) (Port, error) {
	var wrapper struct {
		Port Port `json:"port"`
	}

	url := fmt.Sprintf("%s/v2.0/ports/%s", baseURL, portID)

	apiResp, err := callGET(url, token)
	if err != nil {
		return wrapper.Port, fmt.Errorf("failed to fetch port details: %v", err)
	}

	if apiResp.ResponseCode != 200 {
		return wrapper.Port, fmt.Errorf("get port details request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	err = json.Unmarshal([]byte(apiResp.Response), &wrapper)
	if err != nil {
		return wrapper.Port, fmt.Errorf("failed to parse port details response: %v", err)
	}

	return wrapper.Port, nil
}

// DeletePort deletes a port by ID
func DeletePort(baseURL, token, portID string) error {
	url := fmt.Sprintf("%s/v2.0/ports/%s", baseURL, portID)

	apiResp, err := callDELETE(url, token)
	if err != nil {
		return fmt.Errorf("failed to delete port: %v", err)
	}

	if apiResp.ResponseCode != 204 {
		return fmt.Errorf("delete port request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	return nil
}

// GetPortIDByName finds a port by name and returns its ID.
// Returns an error if zero or multiple ports match.
func GetPortIDByName(baseURL, token, portName string) (string, error) {
	if isUuid(portName) {
		return portName, nil
	}

	queryParams := map[string]string{"name": portName}
	resp, err := ListPorts(baseURL, token, queryParams)
	if err != nil {
		return "", err
	}

	if len(resp.Ports) == 0 {
		return "", fmt.Errorf("no port found with name %s", portName)
	}

	if len(resp.Ports) > 1 {
		return "", fmt.Errorf("multiple ports found with name %s", portName)
	}

	return resp.Ports[0].ID, nil
}
