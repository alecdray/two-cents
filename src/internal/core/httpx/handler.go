package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

type ErrorResponseKind int

const (
	ErrorResponseKindNone ErrorResponseKind = iota
	ErrorResponseKindJSON
	ErrorResponseKindComponent
)

type ErrorResponse struct {
	json      *json.RawMessage
	component templ.Component
}

func NewErrorResponse() *ErrorResponse {
	return &ErrorResponse{}
}

func (e ErrorResponse) Kind() ErrorResponseKind {
	if e.JSON() != nil {
		return ErrorResponseKindJSON
	}
	if e.Component() != nil {
		return ErrorResponseKindComponent
	}
	return ErrorResponseKindNone
}

func (e *ErrorResponse) SetJSON(json *json.RawMessage) *ErrorResponse {
	e.json = json
	return e
}

func (e ErrorResponse) JSON() *json.RawMessage {
	return e.json
}

func (e *ErrorResponse) SetComponent(component templ.Component) *ErrorResponse {
	e.component = component
	return e
}

func (e ErrorResponse) Component() templ.Component {
	return e.component
}

type HandleErrorResponseProps struct {
	Status   int
	Err      error
	Response ErrorResponse
}

func HandleErrorResponse(ctx context.Context, w http.ResponseWriter, props HandleErrorResponseProps) {
	if props.Status == 0 {
		props.Status = http.StatusInternalServerError
	}

	w.WriteHeader(props.Status)
	slog.ErrorContext(ctx, "http error", "error", props.Err, "status", props.Status)

	switch props.Response.Kind() {
	case ErrorResponseKindJSON:
		json.NewEncoder(w).Encode(props.Response.JSON())
	case ErrorResponseKindComponent:
		props.Response.Component().Render(ctx, w)
	case ErrorResponseKindNone:
		// Do nothing
	}
}
