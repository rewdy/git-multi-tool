package cmd

// version is the git-multi-tool release version. It's normally set at
// build time via:
//
//	go build -ldflags "-X git-multi-tool/cmd.version=v1.2.3"
//
// and defaults to "dev" for local builds so `gmt --version` always
// prints something sensible.
var version = "dev"

func versionString() string {
	return version
}
