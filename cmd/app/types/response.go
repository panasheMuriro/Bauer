package types

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Code  int    `json:"code"`
	Error string `json:"error,omitempty"`
}

func Success() *Response {
	return &Response{Code: http.StatusOK, Error: ""}
}

func Accepted() *Response {
	return &Response{Code: http.StatusAccepted, Error: ""}
}

func BadRequest(err error) *Response {
	return &Response{Code: http.StatusBadRequest, Error: err.Error()}
}

func Forbidden(err error) *Response {
	return &Response{Code: http.StatusForbidden, Error: err.Error()}
}

func InternalError(err error) *Response {
	return &Response{Code: http.StatusInternalServerError, Error: err.Error()}
}

func NotFound(err error) *Response {
	return &Response{Code: http.StatusNotFound, Error: err.Error()}
}

func (r *Response) Render(w http.ResponseWriter, _ *http.Request) error {
	w.WriteHeader(r.Code)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(r)
}
