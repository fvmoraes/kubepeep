package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const DefaultJSONBodyLimit int64 = 1 << 20

// DecodeStrict implements the shared JSON grammar: one value, no unknown
// fields, no trailing content and a hard byte limit.
func DecodeStrict(w http.ResponseWriter, r *http.Request, destination any, limit int64) error {
	if limit <= 0 {
		limit = DefaultJSONBodyLimit
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return decodeError(err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return decodeError(err)
		}
		return NewHTTPError(http.StatusBadRequest, CodeInvalidJSON, "The JSON body contains trailing content.", nil, nil)
	}
	return nil
}

func decodeError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return NewHTTPError(http.StatusRequestEntityTooLarge, CodeBodyTooLarge, "The request body is too large.", nil, err)
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return NewHTTPError(http.StatusBadRequest, CodeUnknownField, "The JSON body contains an unknown field.", nil, err)
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return NewHTTPError(http.StatusBadRequest, CodeValidationFailed, "A JSON field has an invalid type.", nil, err)
	}
	return NewHTTPError(http.StatusBadRequest, CodeInvalidJSON, "The request body is not valid JSON.", nil, err)
}
