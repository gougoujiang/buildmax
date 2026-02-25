package config

import "os"

// DefaultQuotaTier returns the default quota tier name for new users (from BUILDMAX_DEFAULT_QUOTA_TIER).
// Returns "free_trial" when unset.
func DefaultQuotaTier() string {
	if t := os.Getenv(EnvKeyBuildmaxDefaultQuotaTier); t != "" {
		return t
	}
	return "free_trial"
}
