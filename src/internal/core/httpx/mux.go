package httpx

import (
	"net/http"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/contextx"

	"github.com/google/uuid"
)

type Handler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

type HandlerFunc func(w http.ResponseWriter, r *http.Request)

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}

func WrapHandler(handler http.Handler, middlewares ...Middleware) HandlerFunc {
	return ApplyMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}, middlewares...)
}

type Mux struct {
	mux        *http.ServeMux
	app        app.App
	middleware []Middleware
}

func NewMux(app app.App, middlewares ...Middleware) *Mux {
	return &Mux{
		mux:        http.NewServeMux(),
		app:        app,
		middleware: middlewares,
	}
}

func (wm *Mux) wrapHandler(handler Handler) http.HandlerFunc {
	handlerFunc := ApplyMiddleware(handler.ServeHTTP, wm.middleware...)
	return func(w http.ResponseWriter, r *http.Request) {
		requestId := uuid.New().String()

		ctx := contextx.NewContextX(r.Context()).
			WithApp(wm.app).
			WithRequestId(requestId)

		handlerFunc(w, r.WithContext(ctx))
	}
}

func (wm *Mux) HandleFunc(pattern string, handler Handler, middlewares ...Middleware) {
	handler = ApplyMiddleware(handler.ServeHTTP, middlewares...)
	wm.mux.HandleFunc(pattern, wm.wrapHandler(handler))
}

func (wm *Mux) Handle(pattern string, handler Handler, middlewares ...Middleware) {
	handler = ApplyMiddleware(handler.ServeHTTP, middlewares...)
	wm.mux.Handle(pattern, wm.wrapHandler(handler))
}

func (wm *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wm.mux.ServeHTTP(w, r)
}

func (wm *Mux) Use(pattern string, mux *Mux) {
	wm.Handle(pattern, mux)
}
