package version

import (
	"strings"
)

// Build information. Populated at build-time.
var (
	BuildInformation string

	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
)

// SetBuildInformation splits the provided BuildInformation into seperate
// variables.
func SetBuildInformation() {
	if splitted := strings.Split(BuildInformation, ","); len(splitted) == 5 {
		Version = splitted[0]
		Revision = splitted[1]
		Branch = splitted[2]
		BuildUser = splitted[3]
		BuildDate = splitted[4]
	}
}
