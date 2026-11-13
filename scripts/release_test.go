package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestReleaseMetadata(t *testing.T) {
	for _, version := range []string{"0.0.0", "0.1.81", "1.2.3-rc.1", "1.2.3-alpha", "1.2.3-0", "1.2.3-01a"} {
		t.Run(version, func(t *testing.T) {
			out, err := exec.Command("bash", "release-metadata.sh", version).Output()
			if err != nil {
				t.Fatal(err)
			}
			flags := "prerelease=false\nlatest=true\n"
			if strings.Contains(version, "-") {
				flags = "prerelease=true\nlatest=false\n"
			}
			if want := "version=" + version + "\n" + flags; string(out) != want {
				t.Fatalf("metadata = %q, want %q", out, want)
			}
		})
	}
	for _, version := range []string{
		"", "dev", "v1.2.3", "1.2", "01.2.3", "1.02.3", "1.2.03",
		"1.2.3-", "1.2.3-rc..1", "1.2.3-rc.01", "1.2.3+build.1",
		"1.2.3-rc/1", "1.2.3-rc_1", "1.2.3-rc.1\nlatest=true", "1.2.3;echo bad",
		"1.2.3-" + strings.Repeat("a", 123),
	} {
		t.Run("reject/"+version, func(t *testing.T) {
			out, err := exec.Command("bash", "release-metadata.sh", version).Output()
			if err == nil || len(out) != 0 {
				t.Fatalf("invalid version produced metadata: output %q, error %v", out, err)
			}
		})
	}
}

// Run the actual workflow validation step without GitHub or a publication token.
func TestReleaseTagValidation(t *testing.T) {
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(readReleaseFile(t, "../.github/workflows/release.yml"), &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["prepare"].Steps {
		if step.ID == "version" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing release version validation step")
	}
	for _, tc := range []struct {
		tag, version string
		valid        bool
	}{
		{"v0.1.81", "0.1.81", true},
		{"v0.1.81-rc.1", "0.1.81-rc.1", true},
		{"v0.1.81", "0.1.80", false},
		{"v0.1.81-rc.1", "0.1.81", false},
		{"v0.1.81+build.1", "0.1.81+build.1", false},
		{"v0.1.81-rc.1\nlatest=true", "0.1.81-rc.1\nlatest=true", false},
	} {
		t.Run(tc.tag+"/"+tc.version, func(t *testing.T) {
			dir := releaseFixture(t)
			writeReleaseFile(t, filepath.Join(dir, "VERSION"), []byte(tc.version+"\n"), 0644)
			output := filepath.Join(dir, "outputs")
			cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", script)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RELEASE_TAG="+tc.tag, "GITHUB_OUTPUT="+output)
			log, err := cmd.CombinedOutput()
			if (err == nil) != tc.valid {
				t.Fatalf("validation returned %v: %s", err, log)
			}
			if tc.valid {
				if !strings.HasPrefix(string(readReleaseFile(t, output)), "version="+tc.version+"\n") {
					t.Fatal("validated version was not passed to publishing jobs")
				}
			} else if data, _ := os.ReadFile(output); len(data) != 0 {
				t.Fatalf("invalid tag produced publishing outputs: %q", data)
			}
		})
	}
}

func TestDockerPublishTags(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		create, use   string
		valid         bool
		tags          []string
	}{
		{"stable", "0.1.81", "0", "0", true, []string{"example/bootimus:0.1.81", "example/bootimus:latest"}},
		{"prerelease", "0.1.81-rc.1", "0", "0", true, []string{"example/bootimus:0.1.81-rc.1"}},
		{"existing builder", "0.1.81-rc.1", "1", "0", true, []string{"example/bootimus:0.1.81-rc.1"}},
		{"builder failure", "0.1.81", "1", "1", false, nil},
		{"invalid version", "dev", "0", "0", false, nil},
		{"invalid prerelease", "0.1.81-rc.01", "0", "0", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := releaseFixture(t)
			// Every docker invocation is intercepted, including buildx setup.
			writeReleaseFile(t, filepath.Join(dir, "docker"), []byte(`#!/bin/sh
printf '%s\000' "$@" >> "$DOCKER_CALLS"
printf '\n' >> "$DOCKER_CALLS"
case "$1 $2" in
    'buildx create') exit "$CREATE_EXIT" ;;
    'buildx use') exit "$USE_EXIT" ;;
esac
`), 0755)
			calls := filepath.Join(dir, "docker-calls")
			cmd := exec.Command("make", "--no-print-directory", "docker-push", "VERSION="+tc.version, "IMAGE=example/bootimus")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"DOCKER_CALLS="+calls, "CREATE_EXIT="+tc.create, "USE_EXIT="+tc.use)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.valid {
				t.Fatalf("make docker-push returned %v: %s", err, out)
			}
			data, _ := os.ReadFile(calls)
			var tags []string
			pushes := 0
			for _, call := range strings.Split(string(data), "\n") {
				args := strings.Split(call, "\x00")
				for i, arg := range args {
					if arg == "-t" && i+1 < len(args) {
						tags = append(tags, args[i+1])
					}
					if arg == "--push" {
						pushes++
					}
				}
			}
			if !reflect.DeepEqual(tags, tc.tags) || (pushes == 1) != tc.valid || pushes > 1 {
				t.Fatalf("tags = %v, pushes = %d, calls = %q", tags, pushes, data)
			}
			if strings.HasPrefix(tc.name, "invalid") && len(data) != 0 {
				t.Fatalf("invalid version reached Docker: %q", data)
			}
		})
	}
}

func TestInvalidVersionBumpDoesNotWrite(t *testing.T) {
	dir := releaseFixture(t)
	cmd := exec.Command("make", "--no-print-directory", "bump", "NEW_VERSION=0.1.81+build.1")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("invalid version was accepted: %s", out)
	}
	if got := string(readReleaseFile(t, filepath.Join(dir, "VERSION"))); got != "0.1.80\n" {
		t.Fatalf("invalid bump changed VERSION to %q", got)
	}
}

func releaseFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	writeReleaseFile(t, filepath.Join(dir, "Makefile"), readReleaseFile(t, "../Makefile"), 0644)
	writeReleaseFile(t, filepath.Join(dir, "scripts/release-metadata.sh"), readReleaseFile(t, "release-metadata.sh"), 0755)
	writeReleaseFile(t, filepath.Join(dir, "VERSION"), []byte("0.1.80\n"), 0644)
	return dir
}

func readReleaseFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeReleaseFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
