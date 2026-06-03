package e2ekit

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type RunningApp struct {
	Container testcontainers.Container
	BaseURL   string
}

type Builder struct {
	t   *testing.T
	ctx context.Context

	image string
	build *BuildConfig

	env        map[string]string
	cmd        []string
	ports      []string
	networks   []string
	extraHosts []string
	shmSize    int64

	waitStrategy WaitStrategy
	rewriters    []EnvRewriter

	logOnFail         bool
	logOnFailMaxBytes int
}

func New(t *testing.T) *Builder { //nolint:thelper // t can be nil when called from TestMain
	if t != nil {
		t.Helper()
	}

	return &Builder{
		t:                 t,
		env:               map[string]string{},
		extraHosts:        []string{"host.docker.internal:host-gateway"},
		waitStrategy:      WaitRunning(30 * time.Second),
		rewriters:         []EnvRewriter{RewriteLocalhostToHostGateway()},
		logOnFail:         true,
		logOnFailMaxBytes: 4000,
	}
}

func (b *Builder) WithContext(ctx context.Context) *Builder {
	b.ctx = ctx
	return b
}

func (b *Builder) WithImage(image string) *Builder {
	b.image = image
	b.build = nil

	return b
}

func (b *Builder) WithDockerfile(build BuildConfig) *Builder {
	b.build = &build
	return b
}

func (b *Builder) WithEnv(env map[string]string) *Builder {
	for k, v := range env {
		b.env[k] = v
	}

	return b
}

func (b *Builder) WithEnvVar(key, value string) *Builder {
	b.env[key] = value
	return b
}

func (b *Builder) WithCmd(cmd ...string) *Builder {
	b.cmd = append([]string{}, cmd...)
	return b
}

func (b *Builder) ExposePort(port int) *Builder {
	p := fmt.Sprintf("%d/tcp", port)
	for _, existing := range b.ports {
		if existing == p {
			return b
		}
	}

	b.ports = append(b.ports, p)

	return b
}

func (b *Builder) WithNetworks(networks ...string) *Builder {
	b.networks = append([]string{}, networks...)
	return b
}

func (b *Builder) WithExtraHosts(extraHosts ...string) *Builder {
	b.extraHosts = append(b.extraHosts, extraHosts...)
	return b
}

func (b *Builder) WithShmSize(bytes int64) *Builder {
	b.shmSize = bytes
	return b
}

func (b *Builder) WithWait(ws WaitStrategy) *Builder {
	if ws != nil {
		b.waitStrategy = ws
	}

	return b
}

func (b *Builder) WithRewriter(r EnvRewriter) *Builder {
	if r != nil {
		b.rewriters = append(b.rewriters, r)
	}

	return b
}

func (b *Builder) DisableDefaultLocalhostRewrite() *Builder {
	out := make([]EnvRewriter, 0, len(b.rewriters))
	for _, r := range b.rewriters {
		if _, ok := r.(localhostToHostGatewayRewriter); ok {
			continue
		}

		out = append(out, r)
	}

	b.rewriters = out

	return b
}

func (b *Builder) WithLogsOnFailure(enabled bool) *Builder {
	b.logOnFail = enabled
	return b
}

func (b *Builder) WithLogsOnFailureMaxBytes(n int) *Builder {
	if n > 0 {
		b.logOnFailMaxBytes = n
	}

	return b
}

func (b *Builder) Run() (*RunningApp, error) {
	if b.t != nil {
		b.t.Helper()
	}

	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	if b.image == "" && b.build == nil {
		return nil, fmt.Errorf("e2ekit: missing image or dockerfile build config")
	}

	// When secrets are configured, pre-build the image using docker CLI
	// because testcontainers doesn't support BuildKit secrets.
	image := b.image
	if b.build != nil && b.build.hasSecrets() {
		builtTag, err := buildImageWithSecrets(ctx, *b.build)
		if err != nil {
			return nil, fmt.Errorf("e2ekit: build with secrets failed: %w", err)
		}

		image = builtTag
		b.build = nil // Clear build config since image is now built
	}

	env := cloneMap(b.env)
	for _, rw := range b.rewriters {
		env = rw.Rewrite(env)
	}

	req := testcontainers.ContainerRequest{
		Image:        image,
		Env:          env,
		Cmd:          b.cmd,
		ExposedPorts: b.ports,
		ExtraHosts:   b.extraHosts,
		Networks:     b.networks,
	}

	if b.shmSize > 0 {
		req.HostConfigModifier = func(hc *container.HostConfig) {
			hc.ShmSize = b.shmSize
		}
	}

	if b.build != nil {
		req.FromDockerfile = testcontainers.FromDockerfile{
			Context:    b.build.ContextDir,
			Dockerfile: b.build.Dockerfile,
			BuildArgs:  b.build.BuildArgs,
		}
	}

	if b.waitStrategy != nil {
		b.waitStrategy.Configure(&req)
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	app := &RunningApp{Container: c}

	if err := b.resolveBaseURL(ctx, c, app); err != nil {
		return nil, err
	}

	if b.logOnFail && b.t != nil {
		b.t.Cleanup(func() {
			if b.t.Failed() {
				dumpRecentLogs(ctx, b.t, c, b.logOnFailMaxBytes)
			}
		})
	}

	return app, nil
}

// resolveBaseURL resolves the container's base URL when exactly one port is exposed.
func (b *Builder) resolveBaseURL(ctx context.Context, c testcontainers.Container, app *RunningApp) error {
	if len(b.ports) != 1 {
		return nil
	}

	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return err
	}

	mp, err := c.MappedPort(ctx, b.ports[0])
	if err != nil {
		_ = c.Terminate(ctx)
		return err
	}

	app.BaseURL = fmt.Sprintf("http://%s:%s", host, mp.Port())

	return nil
}

// BuildConfig configures how to build a Docker image from a Dockerfile.
type BuildConfig struct {
	// ContextDir is the path to the build context directory.
	ContextDir string

	// Dockerfile is the path to the Dockerfile relative to ContextDir.
	// Defaults to "Dockerfile" if empty.
	Dockerfile string

	// BuildArgs are build-time variables passed to docker build.
	BuildArgs map[string]*string

	// Secrets are BuildKit secrets passed to docker build.
	// When secrets are specified, e2ekit uses the docker CLI with BuildKit
	// instead of testcontainers' built-in build (which doesn't support secrets).
	Secrets []BuildSecret

	// Tag is the image tag to use for the built image.
	// If empty, a unique tag is generated automatically.
	// Only used when Secrets are specified.
	Tag string
}

type WaitStrategy interface {
	Configure(req *testcontainers.ContainerRequest)
}

type waitHTTP struct {
	port    int
	path    string
	timeout time.Duration
}

func WaitHTTP(port int, path string, timeout time.Duration) WaitStrategy {
	if path == "" {
		path = "/health"
	}

	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return waitHTTP{port: port, path: path, timeout: timeout}
}

func (w waitHTTP) Configure(req *testcontainers.ContainerRequest) {
	p := fmt.Sprintf("%d/tcp", w.port)
	req.ExposedPorts = uniqueAppend(req.ExposedPorts, p)

	port := p

	req.WaitingFor = wait.ForAll(
		wait.ForListeningPort(port).WithStartupTimeout(w.timeout),
		wait.ForHTTP(w.path).WithPort(port).WithStartupTimeout(w.timeout),
	).WithDeadline(w.timeout)
}

type waitLog struct {
	text    string
	timeout time.Duration
}

func WaitLog(text string, timeout time.Duration) WaitStrategy {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return waitLog{text: text, timeout: timeout}
}

func (w waitLog) Configure(req *testcontainers.ContainerRequest) {
	req.WaitingFor = wait.ForLog(w.text).WithStartupTimeout(w.timeout)
}

type waitPort struct {
	port    int
	timeout time.Duration
}

func WaitPort(port int, timeout time.Duration) WaitStrategy {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return waitPort{port: port, timeout: timeout}
}

func (w waitPort) Configure(req *testcontainers.ContainerRequest) {
	p := fmt.Sprintf("%d/tcp", w.port)
	req.ExposedPorts = uniqueAppend(req.ExposedPorts, p)
	req.WaitingFor = wait.ForListeningPort(p).WithStartupTimeout(w.timeout)
}

type waitRunning struct {
	timeout time.Duration
}

func WaitRunning(timeout time.Duration) WaitStrategy {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return waitRunning{timeout: timeout}
}

func (w waitRunning) Configure(req *testcontainers.ContainerRequest) {
	req.WaitingFor = nil
}

type EnvRewriter interface {
	Rewrite(env map[string]string) map[string]string
}

type localhostToHostGatewayRewriter struct{}

func RewriteLocalhostToHostGateway() EnvRewriter {
	return localhostToHostGatewayRewriter{}
}

func (localhostToHostGatewayRewriter) Rewrite(env map[string]string) map[string]string {
	out := cloneMap(env)
	for k, v := range out {
		out[k] = rewriteLocalhostForContainer(v)
	}

	return out
}

func rewriteLocalhostForContainer(s string) string {
	if s == "" {
		return s
	}

	// Get the host gateway IP (cached, detects IPv4 address to avoid IPv6 issues)
	hostGatewayAddr := HostGatewayIP()

	if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Host != "" {
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" {
			port := u.Port()
			if port != "" {
				u.Host = hostGatewayAddr + ":" + port
			} else {
				u.Host = hostGatewayAddr
			}

			return u.String()
		}

		return s
	}

	// Handle plain hostname values (e.g., MONGO_HOST=localhost)
	if s == "localhost" || s == "127.0.0.1" {
		return hostGatewayAddr
	}

	// Handle plain host:port addresses (e.g., "localhost:6379" for Redis)
	if strings.HasPrefix(s, "localhost:") {
		return strings.Replace(s, "localhost:", hostGatewayAddr+":", 1)
	}

	if strings.HasPrefix(s, "127.0.0.1:") {
		return strings.Replace(s, "127.0.0.1:", hostGatewayAddr+":", 1)
	}

	out := s
	out = strings.ReplaceAll(out, "host=localhost", "host="+hostGatewayAddr)
	out = strings.ReplaceAll(out, "host=127.0.0.1", "host="+hostGatewayAddr)

	out = strings.ReplaceAll(out, "://localhost", "://"+hostGatewayAddr)
	out = strings.ReplaceAll(out, "://127.0.0.1", "://"+hostGatewayAddr)
	out = strings.ReplaceAll(out, "@localhost:", "@"+hostGatewayAddr+":")
	out = strings.ReplaceAll(out, "@127.0.0.1:", "@"+hostGatewayAddr+":")

	return out
}

func dumpRecentLogs(ctx context.Context, t *testing.T, c testcontainers.Container, maxBytes int) {
	t.Helper()

	r, err := c.Logs(ctx)
	if err != nil {
		t.Logf("[e2ekit] failed to read logs: %v", err)
		return
	}
	defer r.Close()

	if maxBytes <= 0 {
		maxBytes = 4000
	}

	buf := make([]byte, maxBytes)

	n, _ := io.ReadFull(r, buf)
	if n <= 0 {
		n2, _ := r.Read(buf)
		if n2 > 0 {
			t.Logf("[e2ekit] container logs (last ~%d bytes):\n%s", n2, string(buf[:n2]))
		}

		return
	}

	t.Logf("[e2ekit] container logs (last ~%d bytes):\n%s", n, string(buf[:n]))
}

func uniqueAppend(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}

	return append(list, v)
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}

	return out
}
