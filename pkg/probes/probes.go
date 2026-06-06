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
	"sync/atomic"
)

// State tracks the readiness state exposed by the probe handler.
type State struct {
	ready atomic.Bool
}

// MarkReady records that the scaler has completed its first successful metrics
// collection and can serve traffic.
func (s *State) MarkReady() {
	s.ready.Store(true)
}

// Ready reports whether the scaler is ready to serve traffic.
func (s *State) Ready() bool {
	return s.ready.Load()
}

// Handler returns an HTTP handler for Kubernetes liveness/readiness probes.
func Handler(state *State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !state.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
