package cmd

import "runtime/debug"

// version is the git-multi-tool release version. Release builds set it at
// build time via:
//
//	go build -ldflags "-X github.com/rewdy/git-multi-tool/cmd.version=v1.2.3"
//
// and defaults to "dev" for local builds so `gmt --version` always
// prints something sensible.
var version = "dev"

// versionString reports the version to the user. `go install
// github.com/rewdy/git-multi-tool/cmd/git-multi-tool@v1.2.3` never passes
// ldflags, so without a fallback every installed copy would still call itself
// "dev". Go records the module version it resolved in the binary; report that
// when no explicit version was baked in. Local builds say "(devel)" there, so
// they keep the "dev" default.
func versionString() string {
	if version != "dev" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
