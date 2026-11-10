package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/facette/natsort"
	"github.com/jessegalley/vhicmd/api"
	"github.com/jessegalley/vhicmd/internal/responseparser"
	"github.com/spf13/cobra"
)

var (
	flagSGName        string
	flagSGDescription string
	flagSGRuleGroup   string
	flagSGRuleDir     string
	flagSGRuleProto   string
	flagSGRulePortMin int
	flagSGRulePortMax int
	flagSGRuleRemote  string
	flagSGRuleEther   string
)

var listSecurityGroupsCmd = &cobra.Command{
	Use:     "security-groups",
	Aliases: []string{"sg", "sgs", "secgroups", "security-group"},
	Short:   "List security groups",
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		resp, err := api.ListSecurityGroups(networkURL, tok.Value, nil)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(resp.SecurityGroups, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		sort.Slice(resp.SecurityGroups, func(i, j int) bool {
			return natsort.Compare(resp.SecurityGroups[i].Name, resp.SecurityGroups[j].Name)
		})
		var rows [][]string
		for _, g := range resp.SecurityGroups {
			rows = append(rows, []string{
				responseparser.Name(g.Name),
				g.ID,
				fmt.Sprintf("%d", len(g.Rules)),
				g.Description,
			})
		}
		responseparser.PrintTable([]string{"NAME", "ID", "RULES", "DESCRIPTION"}, rows)
		return nil
	},
}

var securityGroupDetailsCmd = &cobra.Command{
	Use:     "security-group [id_or_name]",
	Aliases: []string{"sg", "secgroup"},
	Short:   "Show details and rules of a security group",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		groupID, err := api.GetSecurityGroupIDByName(networkURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		g, err := api.GetSecurityGroupDetails(networkURL, tok.Value, groupID)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(g, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		rows := [][]string{
			{"ID", g.ID},
			{"Name", responseparser.Name(g.Name)},
			{"Description", stringOrNone(g.Description)},
		}
		if g.ProjectID != "" {
			rows = append(rows, []string{"Project ID", g.ProjectID})
		}
		if g.CreatedAt != "" {
			rows = append(rows, []string{"Created", g.CreatedAt})
		}
		if g.UpdatedAt != "" {
			rows = append(rows, []string{"Updated", g.UpdatedAt})
		}
		responseparser.PrintFieldTable("Security Group Info", rows)

		fmt.Println("\nRules:")
		responseparser.PrintTable(
			[]string{"Rule ID", "Direction", "EtherType", "Protocol", "Ports", "Remote"},
			securityGroupRuleRows(g.Rules))
		return nil
	},
}

// securityGroupRuleRows renders rules sorted by direction, with "any" for unset fields.
func securityGroupRuleRows(rules []api.SecurityGroupRule) [][]string {
	sorted := append([]api.SecurityGroupRule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Direction < sorted[j].Direction })

	var rows [][]string
	for _, r := range sorted {
		ports := "any"
		switch {
		case r.PortRangeMin != nil && r.PortRangeMax != nil && *r.PortRangeMin != *r.PortRangeMax:
			ports = fmt.Sprintf("%d-%d", *r.PortRangeMin, *r.PortRangeMax)
		case r.PortRangeMin != nil:
			ports = fmt.Sprintf("%d", *r.PortRangeMin)
		case r.PortRangeMax != nil:
			ports = fmt.Sprintf("%d", *r.PortRangeMax)
		}
		remote := "any"
		if r.RemoteIPPrefix != "" {
			remote = r.RemoteIPPrefix
		} else if r.RemoteGroupID != "" {
			remote = "group " + r.RemoteGroupID
		}
		protocol := r.Protocol
		if protocol == "" {
			protocol = "any"
		}
		rows = append(rows, []string{r.ID, r.Direction, r.EtherType, protocol, ports, remote})
	}
	return rows
}

var createSecurityGroupCmd = &cobra.Command{
	Use:     "security-group",
	Aliases: []string{"sg", "secgroup"},
	Short:   "Create a security group",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		g, err := api.CreateSecurityGroup(networkURL, tok.Value, flagSGName, flagSGDescription)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(g, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		fmt.Printf("Security group created: ID: %s, Name: %s, Rules: %d\n", g.ID, g.Name, len(g.Rules))
		return nil
	},
}

var createSecurityGroupRuleCmd = &cobra.Command{
	Use:     "security-group-rule",
	Aliases: []string{"sg-rule", "secgroup-rule"},
	Short:   "Add a rule to a security group",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rule, err := buildSecurityGroupRule(cmd)
		if err != nil {
			return err
		}

		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		rule.SecurityGroupID, err = api.GetSecurityGroupIDByName(networkURL, tok.Value, flagSGRuleGroup)
		if err != nil {
			return err
		}

		created, err := api.CreateSecurityGroupRule(networkURL, tok.Value, rule)
		if err != nil {
			return err
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(created, "", "  ")
			fmt.Println(string(b))
			return nil
		}

		row := securityGroupRuleRows([]api.SecurityGroupRule{created})[0]
		fmt.Printf("Security group rule created: ID: %s, Group: %s, %s %s, protocol %s, ports %s, remote %s\n",
			created.ID, rule.SecurityGroupID, row[1], row[2], row[3], row[4], row[5])
		return nil
	},
}

// buildSecurityGroupRule checks the rule flags and returns the request without the group ID.
func buildSecurityGroupRule(cmd *cobra.Command) (api.SecurityGroupRuleRequest, error) {
	var rule api.SecurityGroupRuleRequest

	rule.Direction = strings.ToLower(flagSGRuleDir)
	if rule.Direction != "ingress" && rule.Direction != "egress" {
		return rule, fmt.Errorf("--direction must be ingress or egress")
	}

	switch strings.ToLower(flagSGRuleEther) {
	case "ipv4":
		rule.EtherType = "IPv4"
	case "ipv6":
		rule.EtherType = "IPv6"
	default:
		return rule, fmt.Errorf("--ethertype must be IPv4 or IPv6")
	}

	rule.Protocol = strings.ToLower(flagSGRuleProto)
	rule.RemoteIPPrefix = flagSGRuleRemote

	hasMin, hasMax := cmd.Flags().Changed("port-min"), cmd.Flags().Changed("port-max")
	if hasMin || hasMax {
		if rule.Protocol == "" {
			return rule, fmt.Errorf("--port-min and --port-max need --protocol")
		}
		// One bound alone means that single port.
		low, high := flagSGRulePortMin, flagSGRulePortMax
		if !hasMin {
			low = high
		}
		if !hasMax {
			high = low
		}
		if low < 0 || high < 0 || low > 65535 || high > 65535 {
			return rule, fmt.Errorf("ports must be between 0 and 65535")
		}
		if low > high && rule.Protocol != "icmp" && rule.Protocol != "ipv6-icmp" && rule.Protocol != "icmpv6" {
			return rule, fmt.Errorf("--port-min must not be above --port-max")
		}
		rule.PortRangeMin, rule.PortRangeMax = &low, &high
	}
	return rule, nil
}

var deleteSecurityGroupCmd = &cobra.Command{
	Use:     "security-group <id_or_name>",
	Aliases: []string{"sg", "secgroup"},
	Short:   "Delete a security group",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		groupID, err := api.GetSecurityGroupIDByName(networkURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		if err := api.DeleteSecurityGroup(networkURL, tok.Value, groupID); err != nil {
			return err
		}

		fmt.Printf("Security group %s deleted\n", groupID)
		return nil
	},
}

var deleteSecurityGroupRuleCmd = &cobra.Command{
	Use:     "security-group-rule <rule_id>",
	Aliases: []string{"sg-rule", "secgroup-rule"},
	Short:   "Delete a security group rule",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		if err := api.DeleteSecurityGroupRule(networkURL, tok.Value, args[0]); err != nil {
			return err
		}

		fmt.Printf("Security group rule %s deleted\n", args[0])
		return nil
	},
}

// resolveSecurityGroups turns a comma-separated list of security group IDs or names into IDs.
func resolveSecurityGroups(networkURL, csv string) ([]string, error) {
	ids := []string{}
	for _, ref := range strings.Split(csv, ",") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		id, err := api.GetSecurityGroupIDByName(networkURL, tok.Value, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func init() {
	createSecurityGroupCmd.Flags().StringVar(&flagSGName, "name", "", "Name of the security group")
	createSecurityGroupCmd.Flags().StringVar(&flagSGDescription, "description", "", "Description of the security group")
	createSecurityGroupCmd.MarkFlagRequired("name")

	createSecurityGroupRuleCmd.Flags().StringVar(&flagSGRuleGroup, "group", "", "Security group ID or name")
	createSecurityGroupRuleCmd.Flags().StringVar(&flagSGRuleDir, "direction", "", "ingress or egress")
	createSecurityGroupRuleCmd.Flags().StringVar(&flagSGRuleProto, "protocol", "", "Protocol name or number, e.g. tcp, udp, icmp (default: any)")
	createSecurityGroupRuleCmd.Flags().IntVar(&flagSGRulePortMin, "port-min", 0, "First port of the range (ICMP: type)")
	createSecurityGroupRuleCmd.Flags().IntVar(&flagSGRulePortMax, "port-max", 0, "Last port of the range (ICMP: code)")
	createSecurityGroupRuleCmd.Flags().StringVar(&flagSGRuleRemote, "remote-ip", "", "Remote CIDR (default: any)")
	createSecurityGroupRuleCmd.Flags().StringVar(&flagSGRuleEther, "ethertype", "IPv4", "IPv4 or IPv6")
	createSecurityGroupRuleCmd.MarkFlagRequired("group")
	createSecurityGroupRuleCmd.MarkFlagRequired("direction")

	listCmd.AddCommand(listSecurityGroupsCmd)
	detailsCmd.AddCommand(securityGroupDetailsCmd)
	createCmd.AddCommand(createSecurityGroupCmd)
	createCmd.AddCommand(createSecurityGroupRuleCmd)
	deleteCmd.AddCommand(deleteSecurityGroupCmd)
	deleteCmd.AddCommand(deleteSecurityGroupRuleCmd)
}
