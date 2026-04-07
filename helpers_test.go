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

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func makeOC(obj map[string]interface{}) resource.ObservedComposed {
	c := composed.New()
	c.Object = obj
	return resource.ObservedComposed{Resource: c}
}

// ---------------------------------------------------------------------------
// saExtName
// ---------------------------------------------------------------------------

func TestSaExtName(t *testing.T) {
	tests := []struct {
		name string
		n    string
		uid  string
		want string
	}{
		{"simple", "mystore", "abc12345-dead", ""},
		{"uppercase", "MyStore", "abc12345", ""},
		{"name trunc", "averylongnamethatshouldbetruncated", "abc12345", ""},
		{"uid trunc", "store", "12345678abcdef", ""},
		{"empty uid", "store", "", ""},
		{"empty name", "", "abc12345", ""},
	}
	tests[0].want = "mystore" + "abc12345"[:8]
	tests[1].want = "mystore" + "abc12345"[:8]
	tests[2].want = "averylongnametha" + "abc12345"[:8]
	tests[3].want = "store" + "12345678"
	tests[4].want = "store"
	tests[5].want = "abc12345"

	for _, tc := range tests {
		got := saExtName(tc.n, tc.uid)
		if got != tc.want {
			t.Errorf("%s: saExtName(%q,%q) = %q, want %q", tc.name, tc.n, tc.uid, got, tc.want)
		}
	}
}

func TestSaExtName_MaxLength(t *testing.T) {
	got := saExtName("averylongnamethatshouldbetruncated", "aaaabbbbccccddddeeee")
	if len(got) > 24 {
		t.Errorf("saExtName returned %d chars (max 24): %q", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// isSAReady
// ---------------------------------------------------------------------------

func TestIsSAReady(t *testing.T) {
	tests := []struct {
		name string
		obs  observed
		want bool
	}{
		{"empty", observed{}, false},
		{"key missing", observed{"other": makeOC(map[string]interface{}{})}, false},
		{
			"no conditions",
			observed{"storage-account": makeOC(map[string]interface{}{
				"status": map[string]interface{}{},
			})},
			false,
		},
		{
			"Ready=False",
			observed{"storage-account": makeOC(map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": "False"},
					},
				},
			})},
			false,
		},
		{
			"Ready=True",
			observed{"storage-account": makeOC(map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready", "status": "True"},
					},
				},
			})},
			true,
		},
	}
	for _, tc := range tests {
		got := isSAReady(tc.obs)
		if got != tc.want {
			t.Errorf("%s: isSAReady() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// primaryBlobEndpoint
// ---------------------------------------------------------------------------

func TestPrimaryBlobEndpoint(t *testing.T) {
	tests := []struct {
		name string
		obs  observed
		want string
	}{
		{"empty", observed{}, ""},
		{"key missing", observed{"other": makeOC(map[string]interface{}{})}, ""},
		{
			"no atProvider",
			observed{"storage-account": makeOC(map[string]interface{}{
				"status": map[string]interface{}{},
			})},
			"",
		},
		{
			"endpoint present",
			observed{"storage-account": makeOC(map[string]interface{}{
				"status": map[string]interface{}{
					"atProvider": map[string]interface{}{
						"primaryBlobEndpoint": "https://myaccount.blob.core.windows.net/",
					},
				},
			})},
			"https://myaccount.blob.core.windows.net/",
		},
	}
	for _, tc := range tests {
		got := primaryBlobEndpoint(tc.obs)
		if got != tc.want {
			t.Errorf("%s: primaryBlobEndpoint() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// rookReady
// ---------------------------------------------------------------------------

func TestCephObjectStoreReady(t *testing.T) {
	makeStore := func(phase string) map[string]interface{} {
		store := map[string]interface{}{}
		_ = unstructured.SetNestedField(store, phase, "status", "phase")
		return store
	}

	tests := []struct {
		name  string
		store map[string]interface{}
		want  bool
	}{
		{"nil store returns false", nil, false},
		{"phase=Ready returns true", makeStore("Ready"), true},
		{"phase=Connecting returns false", makeStore("Connecting"), false},
	}

	for _, tc := range tests {
		got := cephObjectStoreReady(tc.store)
		if got != tc.want {
			t.Errorf("%s: cephObjectStoreReady() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFetchFirstCephObjectStore(t *testing.T) {
	ctx := context.Background()

	makeStore := func(name, ns, phase string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "ceph.rook.io", Version: "v1", Kind: "CephObjectStore",
		})
		u.SetName(name)
		u.SetNamespace(ns)
		_ = unstructured.SetNestedField(u.Object, phase, "status", "phase")
		return u
	}

	tests := []struct {
		name      string
		objects   []ctrlclient.Object
		wantNil   bool
		wantPhase string
	}{
		{"nil client returns nil", nil, true, ""},
		{"no stores returns nil", []ctrlclient.Object{}, true, ""},
		{"returns first store", []ctrlclient.Object{makeStore("ceph-objectstore", "rook-ceph", "Ready")}, false, "Ready"},
		{"first store not Ready", []ctrlclient.Object{makeStore("ceph-objectstore", "rook-ceph", "Connecting")}, false, "Connecting"},
	}

	for _, tc := range tests {
		var cl ctrlclient.Client
		if tc.objects != nil {
			cl = fake.NewClientBuilder().WithObjects(tc.objects...).Build()
		}
		got := fetchFirstCephObjectStore(ctx, cl)
		if tc.wantNil && got != nil {
			t.Errorf("%s: expected nil, got %v", tc.name, got)
			continue
		}
		if !tc.wantNil && got == nil {
			t.Errorf("%s: expected non-nil store", tc.name)
			continue
		}
		if !tc.wantNil {
			phase, _, _ := unstructured.NestedString(got, "status", "phase")
			if phase != tc.wantPhase {
				t.Errorf("%s: phase = %q, want %q", tc.name, phase, tc.wantPhase)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// obcReady
// ---------------------------------------------------------------------------

func TestObcReady(t *testing.T) {
	tests := []struct {
		name string
		obs  observed
		want bool
	}{
		{"empty", observed{}, false},
		{
			"phase Pending",
			observed{"ceph-obc": makeOC(map[string]interface{}{
				"status": map[string]interface{}{"phase": "Pending"},
			})},
			false,
		},
		{
			"phase Bound",
			observed{"ceph-obc": makeOC(map[string]interface{}{
				"status": map[string]interface{}{"phase": "Bound"},
			})},
			true,
		},
	}
	for _, tc := range tests {
		got := obcReady(tc.obs)
		if got != tc.want {
			t.Errorf("%s: obcReady() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// nestedString
// ---------------------------------------------------------------------------

func TestNestedString(t *testing.T) {
	obj := map[string]interface{}{
		"a": map[string]interface{}{"b": "found"},
		"c": 42,
	}
	tests := []struct {
		name   string
		fields []string
		want   string
		wantOK bool
	}{
		{"nested found", []string{"a", "b"}, "found", true},
		{"missing key", []string{"x", "y"}, "", false},
		{"wrong type at leaf", []string{"c"}, "", false},
		{"wrong type mid path", []string{"c", "d"}, "", false},
	}
	for _, tc := range tests {
		got, ok, _ := unstructured.NestedString(obj, tc.fields...)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%s: nestedString() = (%q,%v), want (%q,%v)", tc.name, got, ok, tc.want, tc.wantOK)
		}
	}
}

// ---------------------------------------------------------------------------
// nestedSlice
// ---------------------------------------------------------------------------

func TestNestedSlice(t *testing.T) {
	obj := map[string]interface{}{
		"list": []interface{}{"a", "b"},
		"str":  "notaslice",
	}
	tests := []struct {
		name   string
		fields []string
		wantOK bool
	}{
		{"found", []string{"list"}, true},
		{"missing", []string{"nope"}, false},
		{"wrong type", []string{"str"}, false},
	}
	for _, tc := range tests {
		_, ok, _ := unstructured.NestedSlice(obj, tc.fields...)
		if ok != tc.wantOK {
			t.Errorf("%s: nestedSlice() ok = %v, want %v", tc.name, ok, tc.wantOK)
		}
	}
}
