package adapters

import (
	"net/http"

	"github.com/alecdray/two-cents/src/internal/home/adapters/views"
)

type HttpHandler struct{}

func NewHttpHandler() *HttpHandler {
	return &HttpHandler{}
}

func (h *HttpHandler) Home(w http.ResponseWriter, r *http.Request) {
	views.HomePage().Render(r.Context(), w)
}
