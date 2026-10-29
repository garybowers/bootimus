package bootloaders

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModernBuilderRejectsUnsafeInputs(test *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		test.Skip("bash is unavailable")
	}
	if _, err := exec.LookPath("grep"); err != nil {
		test.Skip("grep is unavailable")
	}
	directory := test.TempDir()
	privateKey := filepath.Join(directory, "private.pem")
	if err := os.WriteFile(privateKey, []byte("-----BEGIN PRIVATE KEY-----\nnot-a-certificate\n"), 0600); err != nil {
		test.Fatal(err)
	}
	bundle := filepath.Join(directory, "bundle.pem")
	if err := os.WriteFile(bundle, []byte("-----BEGIN CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\n"), 0600); err != nil {
		test.Fatal(err)
	}
	for _, scenario := range []struct {
		name       string
		arguments  []string
		message    string
		secureBoot string
	}{
		{"usage", nil, "Usage:", "0"},
		{"existing directory", []string{directory, privateKey}, "already exists", "0"},
		{"private key", []string{filepath.Join(directory, "key-output"), privateKey}, "never private keys", "0"},
		{"certificate bundle", []string{filepath.Join(directory, "bundle-output"), bundle}, "one PEM certificate", "0"},
		{"invalid secure boot mode", nil, "IPXE_SECURE_BOOT must be 0 or 1", "true"},
		{"restricted private key", []string{filepath.Join(directory, "restricted-output"), privateKey}, "never private keys", "1"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			arguments := append([]string{"../scripts/build-modern-bootloaders.sh"}, scenario.arguments...)
			command := exec.Command("bash", arguments...)
			command.Env = append(os.Environ(), "CONTAINER_ENGINE=must-not-run", "TMPDIR="+directory, "IPXE_SECURE_BOOT="+scenario.secureBoot)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), scenario.message) {
				test.Fatalf("unexpected rejection: %v: %s", err, output)
			}
		})
	}
}

func TestModernBuilderDiscardsPreloadedBootstrap(test *testing.T) {
	for _, command := range []string{"bash", "openssl", "grep", "tail"} {
		if _, err := exec.LookPath(command); err != nil {
			test.Skipf("%s is unavailable", command)
		}
	}
	directory := test.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		test.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		test.Fatal(err)
	}
	certificatePath := filepath.Join(directory, "ca.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), 0600); err != nil {
		test.Fatal(err)
	}
	engine := filepath.Join(directory, "engine")
	if err := os.WriteFile(engine, []byte("#!/bin/bash\n[[ $1 == build ]] || exit 99\ncp \"${@: -1}/embed.ipxe\" \"$CAPTURE_EMBED\"\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\nexit 42\n"), 0700); err != nil {
		test.Fatal(err)
	}
	original, err := os.ReadFile("default/embed.ipxe")
	if err != nil {
		test.Fatal(err)
	}
	for _, mode := range []string{"0", "1"} {
		test.Run(mode, func(test *testing.T) {
			capturedEmbed := filepath.Join(directory, "embed-"+mode)
			capturedArgs := filepath.Join(directory, "args-"+mode)
			command := exec.Command("bash", "../scripts/build-modern-bootloaders.sh", filepath.Join(directory, "output-"+mode), certificatePath)
			command.Env = append(os.Environ(), "CONTAINER_ENGINE="+engine, "IPXE_SECURE_BOOT="+mode, "TMPDIR="+directory, "CAPTURE_EMBED="+capturedEmbed, "CAPTURE_ARGS="+capturedArgs)
			output, err := command.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 42 {
				test.Fatalf("builder did not reach the capture engine: %v: %s", err, output)
			}
			embed, err := os.ReadFile(capturedEmbed)
			if err != nil {
				test.Fatal(err)
			}
			if string(embed) != "#!ipxe\nimgfree autoexec.ipxe ||\n"+strings.TrimPrefix(string(original), "#!ipxe\n") {
				test.Fatalf("unexpected embedded bootstrap: %s", embed)
			}
			arguments, err := os.ReadFile(capturedArgs)
			if err != nil || !strings.Contains(string(arguments), "--build-arg\nSECURE_BOOT="+mode+"\n") {
				test.Fatalf("missing build mode: %v: %s", err, arguments)
			}
		})
	}
}
