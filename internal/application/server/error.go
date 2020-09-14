package server

import (
	"net/http"

	"github.com/go-chi/render"
)

// ErrResponse renderer type for handling all sorts of errors.
// swagger:response ErrResponse
type ErrResponse struct {
	HTTPStatusCode int `json:"-"` // http response status code
	// in: body
	Body ErrResponseBody
}

// ErrResponseBody is readable output to application/human about error
type ErrResponseBody struct {
	// user-level status message
	StatusText string `json:"status"`
	// application-level error message, for debugging
	ErrorText string `json:"error,omitempty"`
}

// Render forms output for ErrResponse
func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) {
	render.Status(r, e.HTTPStatusCode)
	render.JSON(w, r, e.Body)
}

// ErrInvalidRequest returns failure due to incorrect request parameters or methods
func ErrInvalidRequest(err error) *ErrResponse {
	return &ErrResponse{
		HTTPStatusCode: 400,
		Body: ErrResponseBody{
			StatusText: "Invalid request.",
			ErrorText:  err.Error(),
		},
	}
}

// ErrRender returns error for rendering
func ErrRender(err error) *ErrResponse {
	return &ErrResponse{
		HTTPStatusCode: 422,
		Body: ErrResponseBody{
			StatusText: "Error rendering response.",
			ErrorText:  err.Error(),
		},
	}
}

// ErrInternal returns internal server error
func ErrInternal(err error) *ErrResponse {
	return &ErrResponse{
		HTTPStatusCode: 500,
		Body: ErrResponseBody{
			StatusText: "Internal Server Error.",
			ErrorText:  err.Error(),
		},
	}
}

// ErrNotFound is 404
var ErrNotFound = &ErrResponse{
	HTTPStatusCode: 404,
	Body: ErrResponseBody{
		StatusText: "Resource not found.",
	},
}
