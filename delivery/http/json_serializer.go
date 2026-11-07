package http

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
)

// MaxJSONRequestBodyBytes bounds a JSON request body decoded through the Echo serializer.
// No SQS endpoint accepts a JSON body today; the bound keeps a future binding from reading
// an arbitrarily large body into memory.
const MaxJSONRequestBodyBytes = 1 << 20

// maxPooledResponseBufferBytes keeps the occasional very large response (full pool dumps)
// from pinning its buffer in the pool; such buffers are left to the garbage collector.
const maxPooledResponseBufferBytes = 1 << 20

var responseBufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// JSONSerializer is an echo.JSONSerializer backed by encoding/json/v2.
//
// Responses use v2 defaults: nil slices and maps encode as [] and {}, omitempty omits only
// values that encode as empty JSON (null, "", {}, []), invalid UTF-8 is an error, and map
// entries are not sorted. Requests are decoded with v2 defaults: field names match case
// sensitively, duplicate names and invalid UTF-8 are rejected, and the body must hold exactly
// one JSON value. Unknown fields are ignored.
type JSONSerializer struct{}

var _ echo.JSONSerializer = JSONSerializer{}

// Serialize encodes i into a buffer before writing it, so an encoding failure leaves the
// response uncommitted and Echo's error handler can still write a complete error response.
// Like Echo's default serializer, the body ends with a newline.
func (JSONSerializer) Serialize(c echo.Context, i any, indent string) error {
	buf, _ := responseBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= maxPooledResponseBufferBytes {
			responseBufferPool.Put(buf)
		}
	}()

	var opts []json.Options
	if indent != "" {
		opts = append(opts, jsontext.WithIndent(indent))
	}
	if err := json.MarshalWrite(buf, i, opts...); err != nil {
		return err
	}
	buf.WriteByte('\n')

	_, err := c.Response().Write(buf.Bytes())
	return err
}

// Deserialize decodes exactly one JSON value from a request body of at most
// MaxJSONRequestBodyBytes. Malformed or semantically invalid JSON is a 400, an oversized
// body a 413; other errors, such as a failed read, are returned unchanged.
func (JSONSerializer) Deserialize(c echo.Context, i any) error {
	body := http.MaxBytesReader(c.Response().Writer, c.Request().Body, MaxJSONRequestBodyBytes)
	return decodeRequestError(json.UnmarshalRead(body, i))
}

func decodeRequestError(err error) error {
	if err == nil {
		return nil
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		msg := fmt.Sprintf("request body exceeds %d bytes", maxBytesErr.Limit)
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, msg).SetInternal(err)
	}

	var (
		syntacticErr *jsontext.SyntacticError
		semanticErr  *json.SemanticError
	)
	switch {
	case errors.As(err, &syntacticErr):
		return echo.NewHTTPError(http.StatusBadRequest, "malformed JSON: "+syntacticErr.Error()).SetInternal(err)
	case errors.As(err, &semanticErr):
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON value: "+semanticErr.Error()).SetInternal(err)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return echo.NewHTTPError(http.StatusBadRequest, "malformed JSON: unexpected end of input").SetInternal(err)
	default:
		return err
	}
}

// NewEcho returns an Echo instance that encodes and decodes JSON with JSONSerializer.
// Production and handler tests both construct Echo through it, so tests observe the
// production wire format.
func NewEcho() *echo.Echo {
	e := echo.New()
	e.JSONSerializer = JSONSerializer{}
	return e
}
