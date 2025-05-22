package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/jessegalley/vhicmd/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var (
	rootCmd = &cobra.Command{
		Use:          "vhicmd",
		Short:        "A command line utility for calling the VHI compute API",
		Long:         ``,
		SilenceUsage: true,
	}

	cfgFile   string
	rcDirFlag string
	tok       api.Token
	debugMode bool
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.AddTemplateFunc("formatCommand", func(name string, aliases []string) string {
		if len(aliases) > 0 {
			return fmt.Sprintf("%s (%s)", name, strings.Join(aliases, " | "))
		}
		return name
	})

	helpTemplate := `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}

Available Commands (aliases):{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad (formatCommand .Name .Aliases) 35}} {{.Short}}{{end}}{{end}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
	rootCmd.SetUsageTemplate(helpTemplate)
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	rootCmd.PersistentFlags().StringP("host", "H", "", "VHI host to connect to")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file (default is $HOME/.vhirc)")
	rootCmd.PersistentFlags().StringVar(&rcDirFlag, "rc", "", "RC directory for config and token (overrides VHICMD_RCDIR)")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" ||
			cmd.Name() == "validate" {
			return nil
		}

		debug, _ := cmd.Flags().GetBool("debug")
		viper.Set("debug", debug)
		debugMode = debug

		hostFlag, _ := cmd.Flags().GetString("host")
		host := hostFlag
		if host == "" {
			host = viper.GetString("host")
		}
		if host == "" {
			return fmt.Errorf("no host found in flags or config. Provide --host or set 'host' in .vhirc")
		}

		// If this is the "auth" command, skip the token loading
		if cmd.Name() == "auth" || (cmd.Parent() != nil && cmd.Parent().Name() == "config") {
			return nil
		}

		var err error
		tok, err = api.LoadTokenStruct(host)
		if err != nil {
			if err.Error() == "token for "+host+" is expired" {
				return fmt.Errorf("the auth token for '%s' is expired; re-authenticate using 'vhicmd auth'", host)
			}
			return fmt.Errorf("no valid auth token found on disk for host '%s'; run 'vhicmd auth' first", host)
		}

		return nil
	}
}

func initConfig() {
	// Handle RC directory
	if rcDirFlag != "" {
		// Set RC dir from flag
		os.Setenv("VHICMD_RCDIR", rcDirFlag)
		// Export to shell session
		fmt.Printf("Run this to save RC dir for THIS shell session:\n")
		fmt.Printf("export VHICMD_RCDIR=%s\n\n", rcDirFlag)
	}

	// If the config flag is set, it overrides any RC directory
	if cfgFile == "" && rcDirFlag != "" {
		// Construct config path from RC directory
		cfgFile = filepath.Join(rcDirFlag, ".vhirc")
	}

	// Initialize token file
	if err := api.InitTokenFile(cfgFile); err != nil {
		fmt.Printf("Error initializing token file: %v\n", err)
		os.Exit(1)
	}

	viper.AutomaticEnv()
	v, err := config.InitConfig(cfgFile)
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
	}
	// Store viper instance if needed
	viper.Reset()
	*viper.GetViper() = *v

	// Debug info if needed
	if debugMode {
		fmt.Printf("Config file: %s\n", v.ConfigFileUsed())
		fmt.Printf("Token file: %s\n", api.TokenFile)
	}
}
