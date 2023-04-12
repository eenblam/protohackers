package main

import (
	"encoding/json"
	"errors"
)

type Request struct {
	Method string `json:"method"`
	Number int    `json:"number"`
	Float  bool   `json:-`
}

type RawRequestInt struct {
	Method *string `json:"method"`
	Number *int    `json:"number"`
}

type RawRequestFloat struct {
	Method *string  `json:"method"`
	Number *float64 `json:"number"`
}

// UnwrapRequest attempts to parse a JSON request, returning a Request or error.
// Parsing is first done into structs of pointers, to catch missing fields,
// which are errors.
// Floats for Request.Number are parsed, but not included in the returned data.
// Making the assumption that floats are never prime, the returned Request
// is given Number=0, Float=true for later handling of the request's primarily.
func UnwrapRequest(readbuf []byte) (*Request, error) {
	// Stages:
	// 1. Try to parse a raw request with an integer
	// 2. On error, try the same with a float
	// 3. In either case, error if either field is missing (nil-valued pointer)
	// 4. If success, return a non-raw Request
	var rawRequest RawRequestInt
	// Try parsing as a Request
	err := json.Unmarshal(readbuf, &rawRequest)
	if err != nil {
		// Are we only failing because it's a float?
		var rawRequestFloat RawRequestFloat
		err2 := json.Unmarshal(readbuf, &rawRequestFloat)
		if err2 != nil {
			// Nope! Return the original parse error
			return nil, err
		}
		if rawRequestFloat.Number == nil || rawRequestFloat.Method == nil {
			return nil, errors.New("Required field missing")
		}
		// Doesn't matter what Number is, since we treat floats as non-prime
		return &Request{*rawRequestFloat.Method, 0, true}, nil
	}
	// Are we unmarshaling with no number field? e.g. `{"method":"isPrime"}
	if rawRequest.Number == nil || rawRequest.Method == nil {
		return nil, errors.New("Required field missing")
	}

	return &Request{*rawRequest.Method, *rawRequest.Number, false}, nil
}

func isValid(request Request) bool {
	if request.Method != "isPrime" {
		return false
	}
	return true
}
