package engine

import (
	"fmt"
	"linha/server/internal/domain"
	"regexp"
	"strconv"
)

var sparkTimeoutPattern = regexp.MustCompile(`^([1-9][0-9]*)(s|m|h|d)$`)

func (c *SparkSettings) normalizeRuntimeSettings() error {
	if c.UI != nil {
		if port := c.UI.Port; port != nil {
			if *port < 1024 || *port > 65535 {
				return domain.Bad("ui.port must be between 1024 and 65535")
			}
			if *port == 4040 {
				c.UI.Port = nil
			}
		}
		if !c.UI.Enabled && c.UI.Port == nil {
			c.UI = nil
		}
	}
	for _, option := range []struct {
		name         string
		value        *string
		defaultValue string
	}{
		{"executorIdleTimeout", &c.Executors.ExecutorIdleTimeout, "60s"},
		{"cachedExecutorIdleTimeout", &c.Executors.CachedExecutorIdleTimeout, "infinity"},
		{"shuffleTrackingTimeout", &c.Executors.ShuffleTrackingTimeout, "infinity"},
	} {
		if *option.value == "" {
			continue
		}
		if *option.value != "infinity" || option.defaultValue != "infinity" {
			parts := sparkTimeoutPattern.FindStringSubmatch(*option.value)
			if parts == nil {
				return domain.Bad("executors." + option.name + " must use a positive integer and s, m, h or d" + infinityHint(option.defaultValue))
			}
			n, err := strconv.ParseInt(parts[1], 10, 64)
			factor := map[string]int64{"s": 1, "m": 60, "h": 3600, "d": 86400}[parts[2]]
			if err != nil || n > 2147483647/factor {
				return domain.Bad("executors." + option.name + " must not exceed 2147483647 seconds")
			}
			*option.value = fmt.Sprintf("%ds", n*factor)
		}
		// Preserve existing normalized configuration and context hashes when an
		// explicit default is equivalent to an omitted setting.
		if *option.value == option.defaultValue {
			*option.value = ""
		}
	}
	return nil
}

func infinityHint(defaultValue string) string {
	if defaultValue == "infinity" {
		return ", or infinity"
	}
	return ""
}

// runtimeConf is shared by SparkApplication submission and legacy driver Pods.
// Callers supply settings that have passed ParseSpark normalization.
func (c SparkSettings) runtimeConf() map[string]string {
	values := map[string]string{
		"spark.scheduler.mode":            "FAIR",
		"spark.ui.enabled":                "false",
		"spark.dynamicAllocation.enabled": strconv.FormatBool(c.Executors.Dynamic),
	}
	if c.UI != nil {
		values["spark.ui.enabled"] = strconv.FormatBool(c.UI.Enabled)
		port := 4040
		if c.UI.Port != nil {
			port = *c.UI.Port
		}
		values["spark.ui.port"] = strconv.Itoa(port)
	}
	if c.Executors.Dynamic {
		values["spark.dynamicAllocation.shuffleTracking.enabled"] = "true"
		for key, value := range map[string]string{
			"spark.dynamicAllocation.executorIdleTimeout":       c.Executors.ExecutorIdleTimeout,
			"spark.dynamicAllocation.cachedExecutorIdleTimeout": c.Executors.CachedExecutorIdleTimeout,
			"spark.dynamicAllocation.shuffleTracking.timeout":   c.Executors.ShuffleTrackingTimeout,
		} {
			if value != "" {
				values[key] = value
			}
		}
	}
	return values
}
