package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The tests run the real main() in a child process of the test binary, the way the app starts it:
// configuration through environment variables, the listening address read from stdout.
const runMainEnv = "SCOPE_K8S_PROXY_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()

		return
	}

	os.Exit(m.Run())
}

func command(args []string, env ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), env...), runMainEnv+"=1")

	return cmd
}

func writeKubeconfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kubeconfig")
	config := `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:1
contexts:
- name: test
  context:
    cluster: test
    user: test
users:
- name: test
  user:
    token: test-token
current-context: test
`

	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// startProxy starts the proxy and returns the lines it printed up to and including the serving line.
func startProxy(t *testing.T, env ...string) []string {
	t.Helper()

	cmd := command(nil, env...)
	stdout, err := cmd.StdoutPipe()

	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	lines := make(chan string)

	go func() {
		scanner := bufio.NewScanner(stdout)

		for scanner.Scan() {
			lines <- scanner.Text()
		}

		close(lines)
	}()

	var printed []string
	timeout := time.After(15 * time.Second)

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("proxy exited before serving; printed %q", printed)
			}

			printed = append(printed, line)

			if strings.HasPrefix(line, "starting to serve on ") {
				return printed
			}
		case <-timeout:
			t.Fatalf("proxy did not start serving; printed %q", printed)
		}
	}
}

var servingLine = regexp.MustCompile(`^starting to serve on 127\.0\.0\.1:(\d+)$`)

func TestVersionPrintsVersionJSON(t *testing.T) {
	out, err := command([]string{"version"}).Output()

	if err != nil {
		t.Fatal(err)
	}

	var data map[string]string

	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("version output is not JSON: %q", out)
	}

	if _, ok := data["gitVersion"]; !ok {
		t.Fatalf("version output has no gitVersion: %q", out)
	}
}

func TestServesOnALocalPortAndNamesItself(t *testing.T) {
	printed := startProxy(t,
		"KUBECONFIG="+writeKubeconfig(t),
		"KUBECONFIG_CONTEXT=test",
		"API_PREFIX=/some-prefix",
	)

	if !strings.HasPrefix(printed[0], "~~ Scope K8s Proxy") {
		t.Fatalf("first line should name the proxy, got %q", printed[0])
	}

	match := servingLine.FindStringSubmatch(printed[len(printed)-1])

	if match == nil {
		t.Fatalf("unexpected serving line %q", printed[len(printed)-1])
	}

	conn, err := net.Dial("tcp", "127.0.0.1:"+match[1])

	if err != nil {
		t.Fatalf("nothing listens on the printed port: %v", err)
	}

	conn.Close()
}

func TestServesTLSWhenGivenACertificate(t *testing.T) {
	cert, key := selfSignedCertificate(t)
	printed := startProxy(t,
		"KUBECONFIG="+writeKubeconfig(t),
		"KUBECONFIG_CONTEXT=test",
		"API_PREFIX=/some-prefix",
		"PROXY_CERT="+cert,
		"PROXY_KEY="+key,
	)

	if !servingLine.MatchString(printed[len(printed)-1]) {
		t.Fatalf("unexpected serving line %q", printed[len(printed)-1])
	}
}

func selfSignedCertificate(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)

	if err != nil {
		t.Fatal(err)
	}

	keyDer, err := x509.MarshalECPrivateKey(privateKey)

	if err != nil {
		t.Fatal(err)
	}

	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})

	return string(cert), string(key)
}
