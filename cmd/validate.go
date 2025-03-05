package cmd

import (
	"fmt"
	"os"

	"github.com/jessegalley/vhicmd/internal/template"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate <template-file>",
	Short: "Validate and preview a template with provided variables",
	Long:  "Validate a template file and show how variables would be replaced",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		templatePath := args[0]

		// Check if template file exists
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			return fmt.Errorf("template file not found: %s", templatePath)
		}

		// Parse the ci-data
		ciData, err := template.ParseKeyValueString(flagValidateCIData)
		if err != nil {
			return fmt.Errorf("error parsing ci-data: %v", err)
		}

		// If no variables provided, show error
		if len(ciData) == 0 {
			return fmt.Errorf("no variables provided, use --ci-data flag")
		}

		// Read template file
		rawTemplate, err := os.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("failed to read template file: %v", err)
		}

		templateString := string(rawTemplate)

		// Extract and display variables from template
		templateVars := template.ExtractVariables(templateString)

		fmt.Println("\n--- Template variables ---")
		if len(templateVars) == 0 {
			fmt.Println("No variables found in template")
		} else {
			for _, v := range templateVars {
				fmt.Printf("  {{%%%s%%}}\n", v)
			}
		}

		// Display provided variables
		fmt.Println("\n--- Provided variables ---")
		for k, v := range ciData {
			fmt.Printf("  %s: %s\n", k, v)
		}

		// Validate template
		validation := template.ValidateTemplate(templateString, ciData)

		// Check for missing variables
		if len(validation.MissingVariables) > 0 {
			fmt.Println("\n⚠️ ERROR: Missing variables ⚠️")
			for _, v := range validation.MissingVariables {
				fmt.Printf("  {{%%%s%%}} is used in template but no value provided\n", v)
			}
		}

		// Check for unused variables
		if len(validation.UnusedVariables) > 0 {
			fmt.Println("\n⚠️ WARNING: Unused variables ⚠️")
			for _, v := range validation.UnusedVariables {
				fmt.Printf("  %s is provided but not used in template\n", v)
			}
		}

		// Preview processed template
		if flagValidatePreview {
			processedTemplate := template.ReplaceVariables(templateString, ciData)

			fmt.Println("\n--- Processed template preview ---")
			fmt.Println(processedTemplate)
		}

		// Final validation result
		fmt.Println("\n--- Validation result ---")
		if validation.Valid {
			fmt.Println("✅ Template is valid (all required variables provided)")
			return nil
		} else {
			fmt.Println("❌ Template is invalid (missing required variables)")
			return fmt.Errorf("validation failed: template has variables with no values provided")
		}
	},
}

var (
	flagValidateCIData  string
	flagValidatePreview bool
)

func init() {
	validateCmd.Flags().StringVar(&flagValidateCIData, "ci-data", "", "Template variables in format key:value,key:value")
	validateCmd.Flags().BoolVar(&flagValidatePreview, "preview", false, "Show preview of processed template")

	validateCmd.MarkFlagRequired("ci-data")

	rootCmd.AddCommand(validateCmd)
}
