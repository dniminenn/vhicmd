package api

import (
	"encoding/json"
	"fmt"
)

// NeutronSecurityGroup represents a security group as neutron returns it.
type NeutronSecurityGroup struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	ProjectID   string              `json:"project_id,omitempty"`
	CreatedAt   string              `json:"created_at,omitempty"`
	UpdatedAt   string              `json:"updated_at,omitempty"`
	Rules       []SecurityGroupRule `json:"security_group_rules"`
}

// SecurityGroupListResponse represents the response for listing security groups.
type SecurityGroupListResponse struct {
	SecurityGroups []NeutronSecurityGroup `json:"security_groups"`
}

// SecurityGroupRuleRequest holds the fields of a new rule. Nil ports and empty strings are left out,
// which neutron reads as "any".
type SecurityGroupRuleRequest struct {
	SecurityGroupID string `json:"security_group_id"`
	Direction       string `json:"direction"`
	EtherType       string `json:"ethertype,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	PortRangeMin    *int   `json:"port_range_min,omitempty"`
	PortRangeMax    *int   `json:"port_range_max,omitempty"`
	RemoteIPPrefix  string `json:"remote_ip_prefix,omitempty"`
}

// ListSecurityGroups fetches the security groups matching the query, every page.
func ListSecurityGroups(networkURL, token string, queryParams map[string]string) (SecurityGroupListResponse, error) {
	result := SecurityGroupListResponse{SecurityGroups: []NeutronSecurityGroup{}}

	listURL := fmt.Sprintf("%s/v2.0/security-groups", networkURL)
	err := listPages(listURL, token, queryParams, "security_groups_links", func(body []byte) error {
		var page SecurityGroupListResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return fmt.Errorf("failed to parse security groups response: %v", err)
		}
		result.SecurityGroups = append(result.SecurityGroups, page.SecurityGroups...)
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("failed to list security groups: %v", err)
	}
	return result, nil
}

// GetSecurityGroupDetails fetches one security group, with its rules, by ID.
func GetSecurityGroupDetails(networkURL, token, groupID string) (NeutronSecurityGroup, error) {
	var wrapper struct {
		SecurityGroup NeutronSecurityGroup `json:"security_group"`
	}

	url := fmt.Sprintf("%s/v2.0/security-groups/%s", networkURL, groupID)

	apiResp, err := callGET(url, token)
	if err != nil {
		return wrapper.SecurityGroup, fmt.Errorf("failed to fetch security group details: %v", err)
	}
	if apiResp.ResponseCode != 200 {
		return wrapper.SecurityGroup, fmt.Errorf("get security group request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.SecurityGroup, fmt.Errorf("failed to parse security group details: %v", err)
	}
	return wrapper.SecurityGroup, nil
}

// CreateSecurityGroup creates a security group.
func CreateSecurityGroup(networkURL, token, name, description string) (NeutronSecurityGroup, error) {
	var wrapper struct {
		SecurityGroup NeutronSecurityGroup `json:"security_group"`
	}

	var request struct {
		SecurityGroup struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		} `json:"security_group"`
	}
	request.SecurityGroup.Name = name
	request.SecurityGroup.Description = description

	apiResp, err := callPOST(fmt.Sprintf("%s/v2.0/security-groups", networkURL), token, request)
	if err != nil {
		return wrapper.SecurityGroup, fmt.Errorf("failed to create security group: %v", err)
	}
	if apiResp.ResponseCode != 201 {
		return wrapper.SecurityGroup, fmt.Errorf("create security group request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.SecurityGroup, fmt.Errorf("failed to parse create security group response: %v", err)
	}
	return wrapper.SecurityGroup, nil
}

// DeleteSecurityGroup deletes a security group by ID.
func DeleteSecurityGroup(networkURL, token, groupID string) error {
	url := fmt.Sprintf("%s/v2.0/security-groups/%s", networkURL, groupID)

	apiResp, err := callDELETE(url, token)
	if err != nil {
		return fmt.Errorf("failed to delete security group: %v", err)
	}
	if apiResp.ResponseCode != 204 {
		return fmt.Errorf("delete security group request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}
	return nil
}

// CreateSecurityGroupRule adds a rule to a security group.
func CreateSecurityGroupRule(networkURL, token string, rule SecurityGroupRuleRequest) (SecurityGroupRule, error) {
	var wrapper struct {
		Rule SecurityGroupRule `json:"security_group_rule"`
	}

	request := struct {
		Rule SecurityGroupRuleRequest `json:"security_group_rule"`
	}{rule}

	apiResp, err := callPOST(fmt.Sprintf("%s/v2.0/security-group-rules", networkURL), token, request)
	if err != nil {
		return wrapper.Rule, fmt.Errorf("failed to create security group rule: %v", err)
	}
	if apiResp.ResponseCode != 201 {
		return wrapper.Rule, fmt.Errorf("create security group rule request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}

	if err := json.Unmarshal([]byte(apiResp.Response), &wrapper); err != nil {
		return wrapper.Rule, fmt.Errorf("failed to parse create security group rule response: %v", err)
	}
	return wrapper.Rule, nil
}

// DeleteSecurityGroupRule deletes a security group rule by ID.
func DeleteSecurityGroupRule(networkURL, token, ruleID string) error {
	url := fmt.Sprintf("%s/v2.0/security-group-rules/%s", networkURL, ruleID)

	apiResp, err := callDELETE(url, token)
	if err != nil {
		return fmt.Errorf("failed to delete security group rule: %v", err)
	}
	if apiResp.ResponseCode != 204 {
		return fmt.Errorf("delete security group rule request failed [%d]: %s", apiResp.ResponseCode, apiResp.Response)
	}
	return nil
}

// GetSecurityGroupIDByName returns a UUID as is, or the ID of the one security group with that exact name.
func GetSecurityGroupIDByName(networkURL, token, groupName string) (string, error) {
	if isUuid(groupName) {
		return groupName, nil
	}

	groups, err := ListSecurityGroups(networkURL, token, map[string]string{"name": groupName})
	if err != nil {
		return "", err
	}

	var found []NeutronSecurityGroup
	for _, group := range groups.SecurityGroups {
		if group.Name == groupName {
			found = append(found, group)
		}
	}

	if len(found) == 0 {
		return "", fmt.Errorf("no security group found with name %s", groupName)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple security groups found with name %s", groupName)
	}
	return found[0].ID, nil
}
