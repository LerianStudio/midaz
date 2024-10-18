package http

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/gofiber/fiber/v2"
)

// RequestInfo is a struct design to store http access log data.
type RequestInfo struct {
	Method        string
	Username      string
	URI           string
	Referer       string
	RemoteAddress string
	Status        int
	Date          time.Time
	Duration      time.Duration
	UserAgent     string
	CorrelationID string
	Protocol      string
	Size          int
	Body          string
}

// ResponseMetricsWrapper is a Wrapper responsible for collect the response data such as status code and size
// It implements built-in ResponseWriter interface.
type ResponseMetricsWrapper struct {
	Context    *fiber.Ctx
	StatusCode int
	Size       int
	Body       string
}

// NewRequestInfo creates an instance of RequestInfo.
func NewRequestInfo(c *fiber.Ctx) *RequestInfo {
	username, referer := "-", "-"
	rawURL := string(c.Request().URI().FullURI())

	parsedURL, err := url.Parse(rawURL)
	if err == nil && parsedURL.User != nil {
		if name := parsedURL.User.Username(); name != "" {
			username = name
		}
	}

	if c.Get("Referer") != "" {
		referer = c.Get("Referer")
	}

	body := ""

	if c.Request().Header.ContentLength() > 0 {
		body = string(c.Body())
	}

	return &RequestInfo{
		Method:        c.Method(),
		URI:           c.OriginalURL(),
		Username:      username,
		Referer:       referer,
		UserAgent:     c.Get(headerUserAgent),
		CorrelationID: c.Get(headerCorrelationID),
		RemoteAddress: c.IP(),
		Protocol:      c.Protocol(),
		Date:          time.Now().UTC(),
		Body:          body,
	}
}

// CLFString produces a log entry format similar to Common Log Format (CLF)
// Ref: https://httpd.apache.org/docs/trunk/logs.html#common
func (r *RequestInfo) CLFString() string {
	return strings.Join([]string{
		r.RemoteAddress,
		"-",
		r.Username,
		`"` + r.Method,
		r.URI,
		`"` + r.Protocol,
		strconv.Itoa(r.Status),
		strconv.Itoa(r.Size),
		r.Referer,
		r.UserAgent,
	}, " ")
}

// String implements fmt.Stringer interface and produces a log entry using RequestInfo.CLFExtendedString.
func (r *RequestInfo) String() string {
	return r.CLFString()
}

func (r *RequestInfo) debugRequestString() string {
	return strings.Join([]string{
		r.CLFString(),
		r.Referer,
		r.UserAgent,
		r.CorrelationID,
		r.Body,
	}, " ")
}

func (r *RequestInfo) debugResponseString(w *ResponseMetricsWrapper) string {
	return strings.Join([]string{
		r.CLFString(),
		r.Referer,
		r.UserAgent,
		r.CorrelationID,
		w.Body,
	}, " ")
}

// FinishRequestInfo calculates the duration of RequestInfo automatically using time.Now()
// It also set StatusCode and Size of RequestInfo passed by ResponseMetricsWrapper.
func (r *RequestInfo) FinishRequestInfo(rw *ResponseMetricsWrapper) {
	r.Duration = time.Now().UTC().Sub(r.Date)
	r.Status = rw.StatusCode
	r.Size = rw.Size
}

type logMiddleware struct {
	Logger mlog.Logger
}

// LogMiddlewareOption represents the log middleware function as an implementation.
type LogMiddlewareOption func(l *logMiddleware)

// WithCustomLogger is a functional option for logMiddleware.
func WithCustomLogger(logger mlog.Logger) LogMiddlewareOption {
	return func(l *logMiddleware) {
		l.Logger = logger
	}
}

// buildOpts creates an instance of logMiddleware with options.
func buildOpts(opts ...LogMiddlewareOption) *logMiddleware {
	mid := &logMiddleware{
		Logger: &mlog.GoLogger{},
	}

	for _, opt := range opts {
		opt(mid)
	}

	return mid
}

// WithHTTPLogging is a middleware to log access to http server.
// It logs access log according to Apache Standard Logs which uses Common Log Format (CLF)
// Ref: https://httpd.apache.org/docs/trunk/logs.html#common
func WithHTTPLogging(opts ...LogMiddlewareOption) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == "/health" {
			return c.Next()
		}

		info := NewRequestInfo(c)

		mid := buildOpts(opts...)
		logger := mid.Logger.WithFields(
			headerCorrelationID, info.CorrelationID,
		)

		rw := ResponseMetricsWrapper{
			Context:    c,
			StatusCode: 200,
			Size:       0,
			Body:       "",
		}

		logger.Debug(info.debugRequestString())

		ctx := mlog.ContextWithLogger(c.Context(), logger)

		c.SetUserContext(mlog.ContextWithLogger(ctx, logger))

		info.FinishRequestInfo(&rw)

		logger.Debug(info.debugResponseString(&rw))
		logger.Infoln(info)

		if err := c.Next(); err != nil {
			return err
		}

		return nil
	}
}

// WithGrpcLogging is a gRPC unary interceptor to log access to gRPC server.
func WithGrpcLogging(opts ...LogMiddlewareOption) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		mid := buildOpts(opts...)
		logger := mid.Logger

		ctx = mlog.ContextWithLogger(ctx, logger)

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		logger.Infof("gRPC method: %s, Duration: %s, Error: %v", info.FullMethod, duration, err)

		return resp, err
	}
}
