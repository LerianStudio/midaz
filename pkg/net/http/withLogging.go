package http

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/lib-commons/v2/commons/security"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
	TraceID       string
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
		bodyBytes := c.Body()

		if os.Getenv("LOG_OBFUSCATION_DISABLED") != "true" {
			body = getBodyObfuscatedString(c, bodyBytes, security.DefaultSensitiveFields())
		} else {
			body = string(bodyBytes)
		}
	}

	return &RequestInfo{
		TraceID:       c.Get(constant.HeaderID),
		Method:        c.Method(),
		URI:           c.OriginalURL(),
		Username:      username,
		Referer:       referer,
		UserAgent:     c.Get(constant.HeaderUserAgent),
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
		r.Protocol,
		`"` + r.Method + " " + r.URI + `"`,
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
		r.Body,
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
	Logger log.Logger
}

// LogMiddlewareOption represents the log middleware function as an implementation.
type LogMiddlewareOption func(l *logMiddleware)

// WithCustomLogger is a functional option for logMiddleware.
func WithCustomLogger(logger log.Logger) LogMiddlewareOption {
	return func(l *logMiddleware) {
		l.Logger = logger
	}
}

// buildOpts creates an instance of logMiddleware with options.
func buildOpts(opts ...LogMiddlewareOption) *logMiddleware {
	mid := &logMiddleware{
		Logger: &log.GoLogger{},
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

		if strings.Contains(c.Path(), "swagger") && c.Path() != "/swagger/index.html" {
			return c.Next()
		}

		setRequestHeaderID(c)

		info := NewRequestInfo(c)

		headerID := c.Get(constant.HeaderID)

		mid := buildOpts(opts...)
		logger := mid.Logger.WithFields(
			constant.HeaderID, info.TraceID,
		).WithDefaultMessageTemplate(headerID + constant.LoggerDefaultSeparator)

		rw := ResponseMetricsWrapper{
			Context:    c,
			StatusCode: 200,
			Size:       0,
			Body:       "",
		}

		logger.Info(info.debugRequestString())

		ctx := commons.ContextWithLogger(c.UserContext(), logger)

		c.SetUserContext(ctx)

		info.FinishRequestInfo(&rw)

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
		// Prefer request_id from the gRPC request body when available and valid.
		if rid, ok := getValidBodyRequestID(req); ok {
			// Emit a debug log if overriding a different metadata id
			if prev := getMetadataID(ctx); prev != "" && prev != rid {
				mid := buildOpts(opts...)
				mid.Logger.Debugf("Overriding correlation id from metadata (%s) with body request_id (%s)", prev, rid)
			}
			// Override correlation id to match the body-provided, validated UUID request_id
			ctx = commons.ContextWithHeaderID(ctx, rid)
			// Ensure standardized span attribute is present
			ctx = commons.ContextWithSpanAttributes(ctx, attribute.String("app.request.request_id", rid))
		} else {
			// Fallback to metadata path only if body is empty/invalid or accessor not present
			ctx = setGRPCRequestHeaderID(ctx)
		}

		_, _, reqId, _ := commons.NewTrackingFromContext(ctx)

		mid := buildOpts(opts...)
		logger := mid.Logger.
			WithFields(constant.HeaderID, reqId).
			WithDefaultMessageTemplate(reqId + constant.LoggerDefaultSeparator)

		ctx = commons.ContextWithLogger(ctx, logger)

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		logger.Infof("gRPC method: %s, Duration: %s, Error: %v", info.FullMethod, duration, err)

		return resp, err
	}
}

func setRequestHeaderID(c *fiber.Ctx) {
	headerID := c.Get(constant.HeaderID)

	if commons.IsNilOrEmpty(&headerID) {
		headerID = uuid.New().String()
		c.Set(constant.HeaderID, headerID)
		c.Request().Header.Set(constant.HeaderID, headerID)
		c.Response().Header.Set(constant.HeaderID, headerID)
	}

	ctx := commons.ContextWithHeaderID(c.UserContext(), headerID)
	c.SetUserContext(ctx)
}

func setGRPCRequestHeaderID(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		headerID := md.Get(constant.MetadataID)
		if len(headerID) > 0 && !commons.IsNilOrEmpty(&headerID[0]) {
			return commons.ContextWithHeaderID(ctx, headerID[0])
		}
	}

	// If metadata is not present, or if the header ID is missing or empty, generate a new one.
	return commons.ContextWithHeaderID(ctx, uuid.New().String())
}

func getBodyObfuscatedString(c *fiber.Ctx, bodyBytes []byte, fieldsToObfuscate []string) string {
	contentType := c.Get("Content-Type")

	var obfuscatedBody string

	if strings.Contains(contentType, "application/json") {
		obfuscatedBody = handleJSONBody(bodyBytes, fieldsToObfuscate)
	} else if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		obfuscatedBody = handleURLFormBody(c, fieldsToObfuscate)
	} else if strings.Contains(contentType, "multipart/form-data") {
		obfuscatedBody = handleMultipartFormBody(c, fieldsToObfuscate)
	} else {
		obfuscatedBody = string(bodyBytes)
	}

	return obfuscatedBody
}

func handleJSONBody(bodyBytes []byte, fieldsToObfuscate []string) string {
	var bodyData map[string]any
	if err := json.Unmarshal(bodyBytes, &bodyData); err != nil {
		return string(bodyBytes)
	}

	for _, field := range fieldsToObfuscate {
		if _, exists := bodyData[field]; exists {
			bodyData[field] = constant.ObfuscatedValue
		}
	}

	updatedBody, err := json.Marshal(bodyData)
	if err != nil {
		return string(bodyBytes)
	}

	return string(updatedBody)
}

func handleURLFormBody(c *fiber.Ctx, fieldsToObfuscate []string) string {
	formData := c.AllParams()

	for _, field := range fieldsToObfuscate {
		if value := c.FormValue(field); value != "" {
			formData[field] = constant.ObfuscatedValue
		}
	}

	updatedBody := url.Values{}

	for key, value := range formData {
		updatedBody.Set(key, value)
	}

	return updatedBody.Encode()
}

func handleMultipartFormBody(c *fiber.Ctx, fieldsToObfuscate []string) string {
	formData := c.AllParams()
	updatedBody := url.Values{}

	for _, field := range fieldsToObfuscate {
		if _, exists := formData[field]; exists {
			formData[field] = constant.ObfuscatedValue
		}
	}

	for key, value := range formData {
		updatedBody.Set(key, value)
	}

	return updatedBody.Encode()
}

// getValidBodyRequestID extracts and validates the request_id from the gRPC request body.
// Returns (id, true) when present and valid UUID; otherwise ("", false).
func getValidBodyRequestID(req any) (string, bool) {
	if r, ok := req.(interface{ GetRequestId() string }); ok {
		if rid := strings.TrimSpace(r.GetRequestId()); rid != "" && commons.IsUUID(rid) {
			return rid, true
		}
	}

	return "", false
}

// getMetadataID extracts a correlation id from incoming gRPC metadata if present.
func getMetadataID(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok && md != nil {
		headerID := md.Get(constant.MetadataID)
		if len(headerID) > 0 && !commons.IsNilOrEmpty(&headerID[0]) {
			return headerID[0]
		}
	}

	return ""
}
