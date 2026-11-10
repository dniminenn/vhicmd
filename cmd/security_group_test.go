package cmd

import (
	"testing"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

// ruleFlags builds the rule request from command-line arguments, without any API call.
func ruleFlags(t *testing.T, args ...string) (api.SecurityGroupRuleRequest, error) {
	t.Helper()
	flagSGRuleDir, flagSGRuleProto, flagSGRuleRemote, flagSGRuleEther = "", "", "", "IPv4"
	flagSGRulePortMin, flagSGRulePortMax = 0, 0

	c := &cobra.Command{}
	c.Flags().StringVar(&flagSGRuleDir, "direction", "", "")
	c.Flags().StringVar(&flagSGRuleProto, "protocol", "", "")
	c.Flags().IntVar(&flagSGRulePortMin, "port-min", 0, "")
	c.Flags().IntVar(&flagSGRulePortMax, "port-max", 0, "")
	c.Flags().StringVar(&flagSGRuleRemote, "remote-ip", "", "")
	c.Flags().StringVar(&flagSGRuleEther, "ethertype", "IPv4", "")
	if err := c.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	return buildSecurityGroupRule(c)
}

func TestBuildSecurityGroupRule(t *testing.T) {
	rule, err := ruleFlags(t, "--direction", "Ingress", "--protocol", "TCP", "--port-min", "22", "--remote-ip", "10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	if rule.Direction != "ingress" || rule.Protocol != "tcp" || rule.EtherType != "IPv4" || rule.RemoteIPPrefix != "10.0.0.0/8" {
		t.Errorf("rule = %+v", rule)
	}
	if rule.PortRangeMin == nil || rule.PortRangeMax == nil || *rule.PortRangeMin != 22 || *rule.PortRangeMax != 22 {
		t.Errorf("a lone --port-min must mean that one port, got %v-%v", rule.PortRangeMin, rule.PortRangeMax)
	}

	rule, err = ruleFlags(t, "--direction", "egress", "--ethertype", "ipv6")
	if err != nil {
		t.Fatal(err)
	}
	if rule.EtherType != "IPv6" || rule.Protocol != "" || rule.PortRangeMin != nil || rule.PortRangeMax != nil {
		t.Errorf("rule = %+v, want any protocol and any port", rule)
	}

	for name, args := range map[string][]string{
		"direction":         {"--direction", "sideways"},
		"ethertype":         {"--direction", "ingress", "--ethertype", "ipx"},
		"ports no protocol": {"--direction", "ingress", "--port-min", "80"},
		"reversed range":    {"--direction", "ingress", "--protocol", "tcp", "--port-min", "443", "--port-max", "80"},
		"port too high":     {"--direction", "ingress", "--protocol", "udp", "--port-max", "70000"},
	} {
		if _, err := ruleFlags(t, args...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSecurityGroupRuleRows(t *testing.T) {
	low, high := 80, 443
	rows := securityGroupRuleRows([]api.SecurityGroupRule{
		{ID: "r1", Direction: "ingress", EtherType: "IPv4", Protocol: "tcp", PortRangeMin: &low, PortRangeMax: &high, RemoteIPPrefix: "0.0.0.0/0"},
		{ID: "r2", Direction: "egress", EtherType: "IPv6", RemoteGroupID: "sg-9"},
		{ID: "r3", Direction: "ingress", EtherType: "IPv4", Protocol: "tcp", PortRangeMin: &low, PortRangeMax: &low},
	})
	want := [][]string{
		{"r2", "egress", "IPv6", "any", "any", "group sg-9"},
		{"r1", "ingress", "IPv4", "tcp", "80-443", "0.0.0.0/0"},
		{"r3", "ingress", "IPv4", "tcp", "80", "any"},
	}
	for i := range want {
		for j := range want[i] {
			if rows[i][j] != want[i][j] {
				t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
				break
			}
		}
	}
}
