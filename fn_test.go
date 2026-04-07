// Copyright 2026 swengin.io
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func mustNewStruct(m map[string]interface{}) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(err)
	}
	return s
}

func TestRunFunction_SetsHelpersInContext(t *testing.T) {
	f := &Function{}
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: mustNewStruct(map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-store",
						"uid":  "aabbccdd-1234-5678-abcd-ef0123456789",
					},
					"spec": map[string]interface{}{
						"region":            "canadaeast",
						"resourceGroupName": "my-rg",
					},
				}),
			},
		},
	}
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("RunFunction error: %v", err)
	}
	if rsp.Context == nil {
		t.Fatal("expected non-nil context")
	}
	v, ok := rsp.Context.Fields["swengin.io/helpers"]
	if !ok {
		t.Fatal("swengin.io/helpers key not set in context")
	}
	h := v.GetStructValue()
	if h == nil {
		t.Fatal("helpers value is not a struct")
	}
	for _, key := range []string{"saExtName", "isSAReady", "primaryBlobEndpoint", "cephObjectStoreReady", "obcReady"} {
		if _, exists := h.Fields[key]; !exists {
			t.Errorf("missing key %q in helpers", key)
		}
	}
	gotName := h.Fields["saExtName"].GetStringValue()
	wantName := saExtName("my-store", "aabbccdd-1234-5678-abcd-ef0123456789")
	if gotName != wantName {
		t.Errorf("saExtName = %q, want %q", gotName, wantName)
	}
}

func TestRunFunction_DefaultGates(t *testing.T) {
	f := &Function{}
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: mustNewStruct(map[string]interface{}{
					"metadata": map[string]interface{}{"name": "test"},
					"spec":     map[string]interface{}{"region": "on-prem"},
				}),
			},
		},
	}
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("RunFunction error: %v", err)
	}
	h := rsp.Context.Fields["swengin.io/helpers"].GetStructValue()
	if h.Fields["rookReady"].GetBoolValue() != false {
		t.Errorf("rookReady should be false until the observer syncs")
	}
	if h.Fields["isSAReady"].GetBoolValue() != false {
		t.Errorf("isSAReady should be false with no observed SA")
	}
	if h.Fields["obcReady"].GetBoolValue() != false {
		t.Errorf("obcReady should be false with no observed OBC")
	}
}
