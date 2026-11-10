package httpclient

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestRequestTimeout(t *testing.T) {
	defer viper.Reset()

	viper.Reset()
	if got := requestTimeout(); got != defaultRequestTimeout {
		t.Errorf("default = %v, want %v", got, defaultRequestTimeout)
	}

	viper.Set("timeout", 300)
	if got := requestTimeout(); got != 300*time.Second {
		t.Errorf("configured = %v, want 5m", got)
	}

	viper.Set("timeout", "45")
	if got := requestTimeout(); got != 45*time.Second {
		t.Errorf("string value = %v, want 45s", got)
	}
}
