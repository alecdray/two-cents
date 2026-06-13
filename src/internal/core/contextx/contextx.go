package contextx

import (
	"context"
	"errors"

	"github.com/alecdray/two-cents/src/internal/core/app"
)

var (
	ErrInvalidValueType = errors.New("invalid type assertion")
	ErrEmptyValue       = errors.New("value is empty")
)

type ctxKey struct {
	name string
}

var (
	ctxKeyApp       = ctxKey{name: "app"}
	ctxKeyRequestId = ctxKey{name: "request_id"}
	ctxUserId       = ctxKey{name: "user_id"}
)

func getContextXValue[T any](ctx ContextX, key ctxKey) (T, error) {
	val := ctx.Value(key)
	if val == nil {
		return *new(T), ErrEmptyValue
	}
	typedVal, ok := val.(T)
	if !ok {
		return *new(T), ErrInvalidValueType
	}
	return typedVal, nil
}

func withContextXValue(ctx ContextX, key ctxKey, value any) ContextX {
	newCtx := context.WithValue(ctx, key, value)
	return NewContextX(newCtx)
}

type ContextX struct {
	context.Context
}

func NewContextX(ctx context.Context) ContextX {
	return ContextX{
		Context: ctx,
	}
}

func (ctx ContextX) App() (app.App, error) {
	return getContextXValue[app.App](ctx, ctxKeyApp)
}

func (ctx ContextX) WithApp(app app.App) ContextX {
	return withContextXValue(ctx, ctxKeyApp, app)
}

func (ctx ContextX) RequestId() (string, error) {
	return getContextXValue[string](ctx, ctxKeyRequestId)
}

func (ctx ContextX) WithRequestId(requestId string) ContextX {
	return withContextXValue(ctx, ctxKeyRequestId, requestId)
}

func (ctx ContextX) UserId() (string, error) {
	return getContextXValue[string](ctx, ctxUserId)
}

func (ctx ContextX) WithUserId(userId string) ContextX {
	return withContextXValue(ctx, ctxUserId, userId)
}
