package api

import (
	"net/http"
	"strings"
	"testing"
)

const securityGroupJSON = `{"id":"sg-1","name":"web","description":"web servers","project_id":"p1","security_group_rules":[` +
	`{"id":"r1","security_group_id":"sg-1","direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":80,"port_range_max":443,"remote_ip_prefix":"0.0.0.0/0","remote_group_id":null},` +
	`{"id":"r2","security_group_id":"sg-1","direction":"egress","ethertype":"IPv6","protocol":null,"port_range_min":null,"port_range_max":null,"remote_ip_prefix":null,"remote_group_id":"sg-9"}]}`

func TestListSecurityGroups(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"security_groups":[`+securityGroupJSON+`]}`)
	resp, err := ListSecurityGroups(srv.URL, "tok", nil)
	if err != nil {
		t.Fatalf("ListSecurityGroups: %v", err)
	}
	rec.check(t, "GET", "/v2.0/security-groups", "")
	if len(resp.SecurityGroups) != 1 {
		t.Fatalf("groups = %+v", resp.SecurityGroups)
	}
	g := resp.SecurityGroups[0]
	if g.Name != "web" || len(g.Rules) != 2 {
		t.Fatalf("group = %+v", g)
	}
	in, out := g.Rules[0], g.Rules[1]
	if in.Protocol != "tcp" || in.PortRangeMin == nil || *in.PortRangeMin != 80 || *in.PortRangeMax != 443 || in.RemoteIPPrefix != "0.0.0.0/0" {
		t.Errorf("ingress rule = %+v", in)
	}
	if out.Protocol != "" || out.PortRangeMin != nil || out.RemoteGroupID != "sg-9" || out.EtherType != "IPv6" {
		t.Errorf("egress rule = %+v", out)
	}

	srv, _ = actionServer(t, http.StatusForbidden, `{"NeutronError":{"message":"not allowed","type":"PolicyNotAuthorized","detail":""}}`)
	if _, err := ListSecurityGroups(srv.URL, "tok", nil); err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}

func TestGetSecurityGroupDetails(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"security_group":`+securityGroupJSON+`}`)
	g, err := GetSecurityGroupDetails(srv.URL, "tok", "sg-1")
	if err != nil {
		t.Fatalf("GetSecurityGroupDetails: %v", err)
	}
	rec.check(t, "GET", "/v2.0/security-groups/sg-1", "")
	if g.ID != "sg-1" || g.Description != "web servers" || len(g.Rules) != 2 {
		t.Errorf("group = %+v", g)
	}

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"Security group sg-1 does not exist","type":"SecurityGroupNotFound","detail":""}}`)
	if _, err := GetSecurityGroupDetails(srv.URL, "tok", "sg-1"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestCreateSecurityGroup(t *testing.T) {
	srv, rec := actionServer(t, http.StatusCreated, `{"security_group":`+securityGroupJSON+`}`)
	g, err := CreateSecurityGroup(srv.URL, "tok", "web", "web servers")
	if err != nil {
		t.Fatalf("CreateSecurityGroup: %v", err)
	}
	rec.check(t, "POST", "/v2.0/security-groups", `{"security_group":{"name":"web","description":"web servers"}}`)
	if g.ID != "sg-1" {
		t.Errorf("group = %+v", g)
	}

	srv, rec = actionServer(t, http.StatusCreated, `{"security_group":`+securityGroupJSON+`}`)
	if _, err := CreateSecurityGroup(srv.URL, "tok", "web", ""); err != nil {
		t.Fatal(err)
	}
	rec.check(t, "POST", "/v2.0/security-groups", `{"security_group":{"name":"web"}}`)

	srv, _ = actionServer(t, http.StatusConflict, `{"NeutronError":{"message":"Quota exceeded for resources: ['security_group'].","type":"OverQuota","detail":""}}`)
	if _, err := CreateSecurityGroup(srv.URL, "tok", "web", ""); err == nil || !strings.Contains(err.Error(), "Quota exceeded") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestDeleteSecurityGroup(t *testing.T) {
	srv, rec := actionServer(t, http.StatusNoContent, "")
	if err := DeleteSecurityGroup(srv.URL, "tok", "sg-1"); err != nil {
		t.Fatalf("DeleteSecurityGroup: %v", err)
	}
	rec.check(t, "DELETE", "/v2.0/security-groups/sg-1", "")

	srv, _ = actionServer(t, http.StatusConflict, `{"NeutronError":{"message":"Security Group sg-1 in use.","type":"SecurityGroupInUse","detail":""}}`)
	if err := DeleteSecurityGroup(srv.URL, "tok", "sg-1"); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestCreateSecurityGroupRule(t *testing.T) {
	low, high := 80, 443
	cases := []struct {
		name string
		rule SecurityGroupRuleRequest
		want string
	}{
		{
			"tcp range",
			SecurityGroupRuleRequest{SecurityGroupID: "sg-1", Direction: "ingress", EtherType: "IPv4", Protocol: "tcp",
				PortRangeMin: &low, PortRangeMax: &high, RemoteIPPrefix: "10.0.0.0/8"},
			`{"security_group_rule":{"security_group_id":"sg-1","direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":80,"port_range_max":443,"remote_ip_prefix":"10.0.0.0/8"}}`,
		},
		{
			"any",
			SecurityGroupRuleRequest{SecurityGroupID: "sg-1", Direction: "egress", EtherType: "IPv6"},
			`{"security_group_rule":{"security_group_id":"sg-1","direction":"egress","ethertype":"IPv6"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := actionServer(t, http.StatusCreated, `{"security_group_rule":{"id":"r1","security_group_id":"sg-1","direction":"ingress","ethertype":"IPv4","protocol":"tcp","port_range_min":80,"port_range_max":443}}`)
			rule, err := CreateSecurityGroupRule(srv.URL, "tok", tc.rule)
			if err != nil {
				t.Fatalf("CreateSecurityGroupRule: %v", err)
			}
			rec.check(t, "POST", "/v2.0/security-group-rules", tc.want)
			if rule.ID != "r1" || rule.SecurityGroupID != "sg-1" {
				t.Errorf("rule = %+v", rule)
			}
		})
	}

	srv, _ := actionServer(t, http.StatusConflict, `{"NeutronError":{"message":"Security group rule already exists. Rule id is r1.","type":"SecurityGroupRuleExists","detail":""}}`)
	if _, err := CreateSecurityGroupRule(srv.URL, "tok", cases[0].rule); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestDeleteSecurityGroupRule(t *testing.T) {
	srv, rec := actionServer(t, http.StatusNoContent, "")
	if err := DeleteSecurityGroupRule(srv.URL, "tok", "r1"); err != nil {
		t.Fatalf("DeleteSecurityGroupRule: %v", err)
	}
	rec.check(t, "DELETE", "/v2.0/security-group-rules/r1", "")

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"Security group rule r1 does not exist","type":"SecurityGroupRuleNotFound","detail":""}}`)
	if err := DeleteSecurityGroupRule(srv.URL, "tok", "r1"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestGetSecurityGroupIDByName(t *testing.T) {
	groups := []named{{"sg-1", "web"}, {"sg-2", "default"}, {"sg-3", "default"}}
	srv, requests := pagedServer(t, "/v2.0/security-groups", "security_groups", groups, exactFilter)

	if id, err := GetSecurityGroupIDByName(srv.URL, "tok", "web"); err != nil || id != "sg-1" {
		t.Errorf("web = %q, %v", id, err)
	}
	if (*requests)[0] != "name=web" {
		t.Errorf("request = %q", (*requests)[0])
	}
	if _, err := GetSecurityGroupIDByName(srv.URL, "tok", "default"); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Errorf("default: err = %v, want a multiple-match error", err)
	}
	if _, err := GetSecurityGroupIDByName(srv.URL, "tok", "missing"); err == nil {
		t.Error("expected an error for no match")
	}
}

func TestUpdatePortSecurityGroups(t *testing.T) {
	groups := []string{"sg-1", "sg-2"}
	srv, rec := actionServer(t, http.StatusOK, `{"port":{"id":"port-1","network_id":"net-1","security_groups":["sg-1","sg-2"]}}`)
	port, err := UpdatePort(srv.URL, "tok", "port-1", PortUpdateFields{SecurityGroups: &groups})
	if err != nil {
		t.Fatalf("UpdatePort: %v", err)
	}
	rec.check(t, "PUT", "/v2.0/ports/port-1", `{"port":{"security_groups":["sg-1","sg-2"]}}`)
	if len(port.SecurityGroups) != 2 {
		t.Errorf("port = %+v", port)
	}

	// Removing every group must send an empty list, not leave the field out.
	none := []string{}
	off := false
	srv, rec = actionServer(t, http.StatusOK, `{"port":{"id":"port-1","network_id":"net-1","port_security_enabled":false}}`)
	if _, err := UpdatePort(srv.URL, "tok", "port-1", PortUpdateFields{SecurityGroups: &none, PortSecurityEnabled: &off}); err != nil {
		t.Fatalf("UpdatePort: %v", err)
	}
	rec.check(t, "PUT", "/v2.0/ports/port-1", `{"port":{"port_security_enabled":false,"security_groups":[]}}`)

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"Security group sg-1 does not exist","type":"SecurityGroupNotFound","detail":""}}`)
	if _, err := UpdatePort(srv.URL, "tok", "port-1", PortUpdateFields{SecurityGroups: &groups}); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}
