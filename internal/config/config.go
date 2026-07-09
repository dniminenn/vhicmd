package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Host     string `mapstructure:"host"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Domain   string `mapstructure:"domain"`
	Project  string `mapstructure:"project"`
	Networks string `mapstructure:"networks"`
	FlavorID string `mapstructure:"flavor_id"`
	ImageID  string `mapstructure:"image_id"`
}

// GetDefaultConfigPath returns the default path for the config file
func GetDefaultConfigPath() (string, error) {
	// Check VHICMD_RCDIR environment variable first
	if rcDir := os.Getenv("VHICMD_RCDIR"); rcDir != "" {
		return filepath.Join(rcDir, ".vhirc"), nil
	}

	// Otherwise use original user's home directory
	var home string
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		out, err := exec.Command("getent", "passwd", sudoUser).Output()
		if err == nil {
			home = strings.Split(string(out), ":")[5]
		}
	}

	// Fallback to regular UserHomeDir if not running with sudo
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(home, ".vhirc"), nil
}

func InitConfig(cfgFile string) (*viper.Viper, error) {
	v := viper.New()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		v.SetConfigType("yaml")
	} else {
		configPath, err := GetDefaultConfigPath()
		if err != nil {
			return nil, err
		}
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
	}

	// touch the file if it doesn't exist, chmod 600
	if _, err := os.Stat(v.ConfigFileUsed()); os.IsNotExist(err) {
		// Create parent directory if needed
		if err := os.MkdirAll(filepath.Dir(v.ConfigFileUsed()), 0700); err != nil {
			return nil, err
		}
		// Create with 0600 directly; the file may hold credentials
		f, err := os.OpenFile(v.ConfigFileUsed(), os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		f.Close()
	} else if fi, err := os.Stat(v.ConfigFileUsed()); err == nil && fi.Mode().Perm()&0077 != 0 {
		// Config may hold credentials; tighten perms on pre-existing files.
		// Best-effort: a shared RC dir may not be chmod-able by this user.
		if err := os.Chmod(v.ConfigFileUsed(), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s is group/world readable and could not be tightened: %v\n", v.ConfigFileUsed(), err)
		}
	}

	// Environment
	v.SetEnvPrefix("VHI")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			return v, nil
		}
		return nil, err
	}

	return v, nil
}
