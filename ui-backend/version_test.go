// version_test.go
package main

import (
	"os"
	"strings"
	"testing"
)

// The release version is stamped at link time: the Dockerfile passes
// -ldflags "-X main.appVersion=${VERSION}" and the release workflow supplies VERSION as
// a build argument. If the variable moves to another package, or either file drifts,
// the image silently reports "dev"; this keeps the three in step.
func TestTheBuildStampsMainAppVersion(t *testing.T) {
	if appVersion == "" {
		t.Fatal("appVersion must have a default (\"dev\") for local builds")
	}
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("%s not found (%v); run from a full checkout", path, err)
		}
		return string(b)
	}
	if df := read("../Dockerfile"); !strings.Contains(df, `-X main.appVersion=${VERSION}`) || !strings.Contains(df, "ARG VERSION") {
		t.Error("Dockerfile no longer stamps main.appVersion from the VERSION build argument")
	}
	if rel := read("../.github/workflows/release.yml"); !strings.Contains(rel, "build-args: VERSION=") {
		t.Error("release workflow no longer passes VERSION to the image build")
	}
}
