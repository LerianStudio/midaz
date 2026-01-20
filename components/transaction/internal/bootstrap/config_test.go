package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed string
		fallback string
		want     string
	}{
		{
			name:     "prefixed non-empty returns prefixed",
			prefixed: "prefixed-value",
			fallback: "fallback-value",
			want:     "prefixed-value",
		},
		{
			name:     "prefixed empty returns fallback",
			prefixed: "",
			fallback: "fallback-value",
			want:     "fallback-value",
		},
		{
			name:     "prefixed non-empty with empty fallback returns prefixed",
			prefixed: "prefixed-value",
			fallback: "",
			want:     "prefixed-value",
		},
		{
			name:     "both empty returns empty",
			prefixed: "",
			fallback: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := envFallback(tt.prefixed, tt.fallback)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed int
		fallback int
		want     int
	}{
		{
			name:     "prefixed non-zero returns prefixed",
			prefixed: 10,
			fallback: 5,
			want:     10,
		},
		{
			name:     "prefixed zero returns fallback",
			prefixed: 0,
			fallback: 5,
			want:     5,
		},
		{
			name:     "prefixed non-zero with zero fallback returns prefixed",
			prefixed: 10,
			fallback: 0,
			want:     10,
		},
		{
			name:     "both zero returns zero",
			prefixed: 0,
			fallback: 0,
			want:     0,
		},
		{
			name:     "negative prefixed returns prefixed",
			prefixed: -5,
			fallback: 10,
			want:     -5,
		},
		{
			name:     "negative fallback used when prefixed is zero",
			prefixed: 0,
			fallback: -10,
			want:     -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := envFallbackInt(tt.prefixed, tt.fallback)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRabbitMQConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		uri   string
		user  string
		pass  string
		host  string
		port  string
		vhost string
		want  string
	}{
		{
			name:  "empty vhost returns connection string without path",
			uri:   "amqp",
			user:  "guest",
			pass:  "guest",
			host:  "localhost",
			port:  "5672",
			vhost: "",
			want:  "amqp://guest:guest@localhost:5672",
		},
		{
			name:  "simple vhost appends path",
			uri:   "amqp",
			user:  "user",
			pass:  "pass",
			host:  "rabbitmq",
			port:  "5672",
			vhost: "production",
			want:  "amqp://user:pass@rabbitmq:5672/production",
		},
		{
			name:  "vhost with slash is URL encoded",
			uri:   "amqp",
			user:  "user",
			pass:  "pass",
			host:  "localhost",
			port:  "5672",
			vhost: "my/vhost",
			want:  "amqp://user:pass@localhost:5672/my%2Fvhost",
		},
		{
			name:  "default vhost slash is URL encoded",
			uri:   "amqp",
			user:  "user",
			pass:  "pass",
			host:  "localhost",
			port:  "5672",
			vhost: "/",
			want:  "amqp://user:pass@localhost:5672/%2F",
		},
		{
			name:  "vhost with spaces is URL encoded",
			uri:   "amqp",
			user:  "user",
			pass:  "pass",
			host:  "localhost",
			port:  "5672",
			vhost: "my vhost",
			want:  "amqp://user:pass@localhost:5672/my%20vhost",
		},
		{
			name:  "amqps protocol works correctly",
			uri:   "amqps",
			user:  "user",
			pass:  "pass",
			host:  "secure-rabbitmq",
			port:  "5671",
			vhost: "secure",
			want:  "amqps://user:pass@secure-rabbitmq:5671/secure",
		},
		{
			name:  "vhost with special characters is URL encoded",
			uri:   "amqp",
			user:  "user",
			pass:  "pass",
			host:  "localhost",
			port:  "5672",
			vhost: "test#vhost%1",
			want:  "amqp://user:pass@localhost:5672/test%23vhost%251",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildRabbitMQConnectionString(tt.uri, tt.user, tt.pass, tt.host, tt.port, tt.vhost)

			assert.Equal(t, tt.want, got)
		})
	}
}
