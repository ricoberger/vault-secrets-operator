package vault

import (
	"context"
	"fmt"

	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type customHeaderMiddleware struct {
	HeaderName  string
	HeaderValue string
}

func (m *customHeaderMiddleware) ID() string { return "CustomHeaderMiddleware" }

func (m *customHeaderMiddleware) HandleBuild(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
	req, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return middleware.BuildOutput{}, middleware.Metadata{}, fmt.Errorf("unexpected request type %T", in.Request)
	}
	req.Header.Set(m.HeaderName, m.HeaderValue)
	return next.HandleBuild(ctx, in)
}
