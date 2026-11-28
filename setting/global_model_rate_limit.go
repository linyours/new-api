package setting

import (
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/common"
)

const GlobalModelRateLimitOptionKey = "GlobalModelRateLimit"

// Bound both configuration size and sliding-window storage. Counters are only
// allocated for configured models, never for arbitrary client-supplied names.
const MaxGlobalModelRPM = 100000
const MaxGlobalModelRateLimitModels = 1000

type GlobalModelRateLimitConfig struct {
	Enabled bool           `json:"enabled"`
	Limits  map[string]int `json:"limits"`
}

var globalModelRateLimit atomic.Pointer[GlobalModelRateLimitConfig]

func ParseGlobalModelRateLimit(value string) (*GlobalModelRateLimitConfig, error) {
	invalid := common.NewMessage("Invalid global model RPM configuration")
	if len(value) > 512*1024 {
		return nil, invalid
	}
	var input struct {
		Enabled *bool           `json:"enabled"`
		Limits  map[string]*int `json:"limits"`
	}
	if err := common.UnmarshalJsonStr(value, &input); err != nil || input.Enabled == nil || input.Limits == nil || len(input.Limits) > MaxGlobalModelRateLimitModels {
		return nil, invalid
	}
	config := &GlobalModelRateLimitConfig{Enabled: *input.Enabled, Limits: make(map[string]int, len(input.Limits))}
	for model, rpm := range input.Limits {
		name := strings.TrimSpace(model)
		if name == "" || !utf8.ValidString(name) || len(name) > 255 || strings.ContainsAny(name, "\r\n\x00") || rpm == nil || *rpm < 0 || *rpm > MaxGlobalModelRPM {
			return nil, invalid
		}
		if _, exists := config.Limits[name]; exists {
			return nil, invalid
		}
		config.Limits[name] = *rpm
	}
	// A JSON map decoder silently overwrites duplicate keys. Reject these too,
	// rather than letting an ambiguous configuration select the looser limit.
	seen := make(map[string]bool)
	duplicate := false
	ojson.Parse(value).ForEach(func(key, entry ojson.Result) bool {
		if seen[key.String()] || (key.String() != "enabled" && key.String() != "limits") {
			duplicate = true
			return false
		}
		seen[key.String()] = true
		if key.String() == "limits" {
			count := 0
			entry.ForEach(func(_, _ ojson.Result) bool { count++; return true })
			duplicate = count != len(input.Limits)
		}
		return !duplicate
	})
	if duplicate {
		return nil, invalid
	}
	return config, nil
}

func UpdateGlobalModelRateLimit(value string) error {
	config, err := ParseGlobalModelRateLimit(value)
	if err != nil {
		return err
	}
	globalModelRateLimit.Store(config)
	return nil
}

func GlobalModelRPM(model string) int {
	config := globalModelRateLimit.Load()
	if config == nil || !config.Enabled {
		return 0
	}
	return config.Limits[model]
}

func GlobalModelRateLimitJSON() string {
	config := globalModelRateLimit.Load()
	if config == nil {
		return `{"enabled":false,"limits":{}}`
	}
	data, _ := common.Marshal(config) // This immutable bool/int map is always JSON-encodable.
	return string(data)
}
