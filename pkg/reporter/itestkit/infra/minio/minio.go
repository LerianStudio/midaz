package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/itestkit"
)

const (
	defaultImage          = "minio/minio:latest"
	defaultStartupTimeout = 60 * time.Second
	defaultAccessKeyID    = "minioadmin"
	defaultSecretKey      = "minioadmin"
	defaultBucket         = "external-data"
	defaultRegion         = "us-east-1"
)

// MinioConfig configures the MinIO S3-compatible object storage container.
type MinioConfig struct {
	// Name is a unique identifier used in container and network alias names.
	Name string

	// Image is the Docker image to use. Defaults to "minio/minio:latest".
	Image string

	// Bucket is the S3 bucket created on startup. Defaults to "external-data".
	Bucket string

	// AccessKeyID is the MinIO root user (access key). Defaults to "minioadmin".
	AccessKeyID string

	// SecretAccessKey is the MinIO root password (secret key). Defaults to "minioadmin".
	SecretAccessKey string

	// Region is the S3 region used when initialising the AWS SDK client. Defaults to "us-east-1".
	Region string

	// Options is a list of functional options for container customisation (e.g. fixed port, image override).
	Options []MinioOption
}

// MinioEndpoint holds the resolved connection details after the container starts.
type MinioEndpoint struct {
	// URL is the full S3 endpoint URL from the host perspective (e.g. "http://localhost:9000").
	URL string
	// Host is the host part of the mapped address.
	Host string
	// Port is the string representation of the mapped port.
	Port string
	// Upstream is the raw host:port address.
	Upstream string
}

// MinioInfra manages a MinIO S3-compatible object storage container for integration tests.
// It implements itestkit.Infra and follows the same lifecycle as other infra components
// (redis, rabbitmq, mongodb, seaweedfs).
type MinioInfra struct {
	cfg          MinioConfig
	container    testcontainers.Container
	endpoint     *MinioEndpoint
	s3Client     *s3.Client
	networkAlias string // set when container joins a shared Docker network
}

// NewMinioInfra creates a new MinioInfra with defaults applied for missing config values.
func NewMinioInfra(cfg MinioConfig) *MinioInfra {
	if cfg.Name == "" {
		cfg.Name = "default"
	}

	if cfg.Image == "" {
		cfg.Image = defaultImage
	}

	if cfg.Bucket == "" {
		cfg.Bucket = defaultBucket
	}

	if cfg.AccessKeyID == "" {
		cfg.AccessKeyID = defaultAccessKeyID
	}

	if cfg.SecretAccessKey == "" {
		cfg.SecretAccessKey = defaultSecretKey
	}

	if cfg.Region == "" {
		cfg.Region = defaultRegion
	}

	return &MinioInfra{cfg: cfg}
}

// Start launches the MinIO container, waits for it to be healthy, and creates the default bucket.
// It implements itestkit.Infra.
func (m *MinioInfra) Start(ctx context.Context, env *itestkit.Env) error {
	opts := defaultMinioOptions(m.cfg.Image)

	for _, opt := range m.cfg.Options {
		if opt != nil {
			opt(opts)
		}
	}

	alias := fmt.Sprintf("minio-%s", m.cfg.Name)

	var networks []string

	networkAliases := map[string][]string{}

	if env != nil && env.Network != "" {
		networks = []string{env.Network}
		networkAliases = map[string][]string{
			env.Network: {alias},
		}

		m.networkAlias = alias
	}

	hostConfigModifiers := opts.hostConfigModifiers

	req := testcontainers.ContainerRequest{
		Image:        opts.image,
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     m.cfg.AccessKeyID,
			"MINIO_ROOT_PASSWORD": m.cfg.SecretAccessKey,
		},
		Cmd: []string{"server", "/data", "--console-address", ":9001"},
		WaitingFor: wait.ForHTTP("/minio/health/live").
			WithPort("9000/tcp").
			WithStartupTimeout(defaultStartupTimeout),
		Networks:       networks,
		NetworkAliases: networkAliases,
		HostConfigModifier: func(hc *container.HostConfig) {
			for _, modifier := range hostConfigModifiers {
				modifier(hc)
			}
		},
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("start minio container: %w", err)
	}

	m.container = c

	host, err := c.Host(ctx)
	if err != nil {
		return fmt.Errorf("get minio host: %w", err)
	}

	port, err := c.MappedPort(ctx, "9000/tcp")
	if err != nil {
		return fmt.Errorf("get minio port: %w", err)
	}

	upstream := fmt.Sprintf("%s:%s", host, port.Port())

	m.endpoint = &MinioEndpoint{
		URL:      fmt.Sprintf("http://%s", upstream),
		Host:     host,
		Port:     port.Port(),
		Upstream: upstream,
	}

	// Initialise S3 client using the external endpoint (host-visible mapped port)
	s3Client, err := m.newS3Client(ctx, m.endpoint.URL)
	if err != nil {
		_ = c.Terminate(ctx)
		return fmt.Errorf("initialise s3 client: %w", err)
	}

	m.s3Client = s3Client

	// Create the default bucket
	if err := m.ensureBucket(ctx, m.cfg.Bucket); err != nil {
		_ = c.Terminate(ctx)
		return fmt.Errorf("create bucket %q: %w", m.cfg.Bucket, err)
	}

	return nil
}

// Endpoint returns the resolved MinioEndpoint. Returns an error if Start has not been called.
func (m *MinioInfra) Endpoint() (MinioEndpoint, error) {
	if m.endpoint == nil {
		return MinioEndpoint{}, fmt.Errorf("minio endpoint not ready: Start has not been called")
	}

	return *m.endpoint, nil
}

// URL returns the S3 endpoint URL visible from the test host (e.g. "http://localhost:9000").
func (m *MinioInfra) URL() (string, error) {
	endpoint, err := m.Endpoint()
	if err != nil {
		return "", err
	}

	return endpoint.URL, nil
}

// HostPort returns the host and port for container-to-container communication.
// When the container is part of a shared Docker network, returns the network alias
// and the internal port (9000) so other containers can reach MinIO by alias name.
// Otherwise returns the host-mapped port normalised for Docker access.
func (m *MinioInfra) HostPort() (host string, port int, err error) {
	// Containers in the same Docker network reach MinIO via the alias and internal port
	if m.networkAlias != "" {
		return m.networkAlias, 9000, nil
	}

	endpoint, err := m.Endpoint()
	if err != nil {
		return "", 0, err
	}

	hostStr, portStr, err := net.SplitHostPort(endpoint.Upstream)
	if err != nil {
		return "", 0, fmt.Errorf("invalid address format %q: %w", endpoint.Upstream, err)
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	return itestkit.NormalizeHost(hostStr), portNum, nil
}

// Bucket returns the name of the S3 bucket created during Start.
func (m *MinioInfra) Bucket() string {
	return m.cfg.Bucket
}

// AccessKeyID returns the configured access key ID.
func (m *MinioInfra) AccessKeyID() string {
	return m.cfg.AccessKeyID
}

// SecretAccessKey returns the configured secret access key.
func (m *MinioInfra) SecretAccessKey() string {
	return m.cfg.SecretAccessKey
}

// Region returns the configured S3 region.
func (m *MinioInfra) Region() string {
	return m.cfg.Region
}

// HeadObject checks whether an object with the given key exists in the default bucket.
// Returns nil if the object exists, or an error (including NotFound) if it does not.
func (m *MinioInfra) HeadObject(ctx context.Context, key string) error {
	if m.s3Client == nil {
		return fmt.Errorf("minio s3 client not ready: Start has not been called")
	}

	_, err := m.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})

	return err
}

// GetObject downloads the object identified by key from the default bucket.
// Returns the raw bytes (which may be encrypted depending on the application configuration).
func (m *MinioInfra) GetObject(ctx context.Context, key string) ([]byte, error) {
	if m.s3Client == nil {
		return nil, fmt.Errorf("minio s3 client not ready: Start has not been called")
	}

	result, err := m.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q from bucket %q: %w", key, m.cfg.Bucket, err)
	}

	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("read object body for %q: %w", key, err)
	}

	return data, nil
}

// Terminate stops and removes the MinIO container.
func (m *MinioInfra) Terminate(ctx context.Context) error {
	if m.container != nil {
		return m.container.Terminate(ctx)
	}

	return nil
}

// InfraKind returns the infrastructure type identifier.
func (m *MinioInfra) InfraKind() string { return "minio" }

// InfraName returns the instance name provided in MinioConfig.
func (m *MinioInfra) InfraName() string { return m.cfg.Name }

// newS3Client creates an AWS SDK v2 S3 client configured for the given MinIO endpoint.
func (m *MinioInfra) newS3Client(ctx context.Context, endpoint string) (*s3.Client, error) {
	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(m.cfg.Region),
		awsConfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(m.cfg.AccessKeyID, m.cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // required for MinIO path-style addressing
	})

	return client, nil
}

// ensureBucket creates the specified bucket if it does not already exist.
// BucketAlreadyOwnedByYou is treated as a success so Start is idempotent
// when re-using containers with fixed ports.
func (m *MinioInfra) ensureBucket(ctx context.Context, bucket string) error {
	_, err := m.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		var bucketOwnedByYou *s3types.BucketAlreadyOwnedByYou
		if errors.As(err, &bucketOwnedByYou) {
			return nil
		}

		return err
	}

	return nil
}
