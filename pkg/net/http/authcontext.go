package http

import (
	"context"

	v3commons "github.com/LerianStudio/lib-commons/v3/commons"
	v4commons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

func bridgeLibAuthContext(ctx context.Context) context.Context {
	_, tracer, headerID, _ := v4commons.NewTrackingFromContext(ctx)

	bridgedCtx := v3commons.ContextWithTracer(ctx, tracer)
	if headerID != "" {
		bridgedCtx = v3commons.ContextWithHeaderID(bridgedCtx, headerID)
	}

	return bridgedCtx
}

// BridgeLibAuthHTTPContext mirrors v4 request-tracking values into the v3 context keys
// still used by lib-auth/v2.
func BridgeLibAuthHTTPContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.SetUserContext(bridgeLibAuthContext(c.UserContext()))
		return c.Next()
	}
}

// BridgeLibAuthGRPCContext mirrors v4 request-tracking values into the v3 context keys
// still used by lib-auth/v2.
func BridgeLibAuthGRPCContext() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(bridgeLibAuthContext(ctx), req)
	}
}
