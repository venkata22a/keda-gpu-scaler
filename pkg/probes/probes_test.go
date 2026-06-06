/*
Copyright 2026 The keda-gpu-scaler Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package probes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler(t *testing.T) {
	var state State
	handler := Handler(&state)

	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "health always ok", path: "/healthz", want: http.StatusOK},
		{name: "ready starts unavailable", path: "/readyz", want: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("%s returned status %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}

	state.MarkReady()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz after MarkReady returned status %d, want %d", rec.Code, http.StatusOK)
	}
}
