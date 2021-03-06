//   Copyright 2017 MSolution.IO
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type bodyEchoTest struct {
	Foo string `json:"foo" req:"nonzero"`
	Bar int    `json:"bar"`
}

func bodyEchoHandler(r *http.Request, a Arguments) (int, interface{}) {
	var body bodyEchoTest
	MustRequestBody(a, &body)
	return http.StatusOK, body
}

func TestEchoHandler(t *testing.T) {
	example := bodyEchoTest{"test", 42}
	arguments := make(Arguments)
	arguments[argumentKeyBody] = reflect.ValueOf(example)
	status, response := bodyEchoHandler(nil, arguments)
	if status != http.StatusOK {
		t.Errorf("Status code should be %d, is %d.", http.StatusOK, status)
	}
	if response != example {
		t.Errorf("Response should be %v, is %v.", example, response)
	}
}

func TestSuccessfulValidation(t *testing.T) {
	example := bodyEchoTest{"test", 42}
	handler := H(bodyEchoHandler).With(
		RequestBody{example},
	)
	bs, _ := json.Marshal(example)
	requestBody := bytes.NewBuffer(bs)
	request := httptest.NewRequest(http.MethodGet, "/", requestBody)
	status, response := handler.Func(nil, request, make(Arguments))
	if status != http.StatusOK {
		t.Errorf("Status code should be %d, is %d.", http.StatusOK, status)
	}
	if response != example {
		t.Errorf("Response should be %v, is %v.", example, response)
	}
}

func TestFailedValidation(t *testing.T) {
	const expectedError = "foo: value is zero"
	example := bodyEchoTest{"", 42}
	handler := H(bodyEchoHandler).With(
		RequestBody{example},
	)
	bs, _ := json.Marshal(example)
	requestBody := bytes.NewBuffer(bs)
	request := httptest.NewRequest(http.MethodGet, "/", requestBody)
	status, response := handler.Func(nil, request, make(Arguments))
	if status != http.StatusBadRequest {
		t.Errorf("Status code should be %d, is %d.", http.StatusBadRequest, status)
	}
	if errorResponse, ok := response.(error); !ok {
		t.Errorf("Response should be error %q, is %v.", expectedError, response)
	} else if errorResponse.Error() != expectedError {
		t.Errorf("Response should be error %q, is error %q.", expectedError, errorResponse.Error())
	}
}

const expectedDoc = `{
	"summary": "",
	"components": {
		"input:body:example": {
			"summary": "input body example",
			"description": "{\n\t\"foo\": \"test\",\n\t\"bar\": 42\n}"
		},
		"input:body:schema": {
			"summary": "input body schema",
			"description": "{\n\t\"foo\": ¡string!,\n\t\"bar\": ¿int?\n}"
		}
	}
}`

func TestDocumentation(t *testing.T) {
	example := bodyEchoTest{"test", 42}
	handler := H(bodyEchoHandler).With(
		RequestBody{example},
	)
	doc, _ := json.MarshalIndent(handler.Documentation, "", "\t")
	sdoc := string(doc)
	if sdoc != expectedDoc {
		t.Errorf("Documentation should be %q, is %q.", expectedDoc, sdoc)
	}
}
