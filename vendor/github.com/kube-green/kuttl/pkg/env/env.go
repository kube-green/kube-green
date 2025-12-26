package env

import (
	"os"
	"strings"
)

// Expand provides OS expansion of defined ENV VARs inside args to commands.  The expansion is limited to what is defined on the OS
// and the variables passed into to the env parameter. To escape a dollar sign, pass in two dollar signs.
func ExpandWithMap(c string, env map[string]string) string {
	// expand $$ -> $
	fullEnv := map[string]string{
		"$": "$",
	}

	// add all OS environment variables to the map
	for _, envVar := range os.Environ() {
		splitVar := strings.SplitN(envVar, "=", 2)
		if len(splitVar) != 2 {
			continue
		}
		fullEnv[splitVar[0]] = splitVar[1]
	}

	// add env parameter variables to map
	for k, v := range env {
		fullEnv[k] = v
	}

	return os.Expand(c, func(s string) string {
		return fullEnv[s]
	})
}

// Expand provides shell expansion similar to ExpandWithMap without the map extension.  It is os.Env only.
func Expand(c string) string {
	return ExpandWithMap(c, nil)
}
