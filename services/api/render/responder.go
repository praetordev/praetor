package render

import (
	"net/http"
	"strconv"

	"github.com/go-chi/render"
)

// PaginatedResponse is the standard envelope for list responses.
type PaginatedResponse struct {
	Items  interface{} `json:"items"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// Render implements the chi.Render interface.
func (resp *PaginatedResponse) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

// PaginationParams holds limit and offset.
type PaginationParams struct {
	Limit  int
	Offset int
}

// ParsePagination extracts limit and offset from the request.
func ParsePagination(r *http.Request) PaginationParams {
	limit := 50
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}
	if limit > 500 {
		limit = 500
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	return PaginationParams{Limit: limit, Offset: offset}
}

// JSON responds with 200 OK and the payload.
func JSON(w http.ResponseWriter, r *http.Request, v interface{}) {
	render.JSON(w, r, v)
}

// Created responds with 201 Created and the payload.
func Created(w http.ResponseWriter, r *http.Request, v interface{}) {
	render.Status(r, http.StatusCreated)
	render.JSON(w, r, v)
}

// ErrorResponse represents a standard error.
type ErrorResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	ErrorText string `json:"error"` // user-facing error message
}

func (e *ErrorResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrInvalidRequest(err error) render.Renderer {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
		ErrorText:      err.Error(),
	}
}

func ErrNotFound(err error) render.Renderer {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusNotFound,
		ErrorText:      "Resource not found",
	}
}

func ErrInternal(err error) render.Renderer {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
		ErrorText:      "Internal server error",
	}
}

func ErrUnauthorized(err error) render.Renderer {
	return &ErrorResponse{
		Err:            err,
		HTTPStatusCode: http.StatusUnauthorized,
		ErrorText:      "Unauthorized",
	}
}
