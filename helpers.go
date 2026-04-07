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
	"regexp"
	"slices"
	"strings"

	"github.com/crossplane/function-sdk-go/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]`)

// checkProviderConfigs checks the API server directly to see whether each
// ClusterProviderConfig exists. Returns existence booleans for inclusion in
// the helpers context map. If cl is nil (e.g. local dev / tests without a
// cluster), both values default to true so the pipeline is not blocked.
func checkProviderConfigs(ctx context.Context, cl ctrlclient.Client) bool {
	if cl == nil {
		return true
	}

	azureObj := &unstructured.Unstructured{}
	azureObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "azure.m.upbound.io",
		Version: "v1beta1",
		Kind:    "ClusterProviderConfig",
	})
	return cl.Get(ctx, types.NamespacedName{Name: "azure-provider"}, azureObj) == nil
}

// saExtName produces a stable, Azure-safe storage account name suffix from
// the XR name and UID.  Rules: lowercase alphanumeric only, max 24 chars,
// unique per XR instance.
//
//   - Take up to 16 chars of the cleaned (lowercased, non-alnum stripped) name.
//   - Append up to 8 chars of the UID (hyphens stripped) for uniqueness.
func saExtName(name, uid string) string {
	cleaned := nonAlphanumeric.ReplaceAllString(strings.ToLower(name), "")
	suffix := strings.ReplaceAll(uid, "-", "")
	return cleaned[:min(len(cleaned), 16)] + suffix[:min(len(suffix), 8)]
}

// s3ProxyEnabled returns the platform policy for S3Proxy deployment,
// falling back to schema default if not configured.
func s3ProxyEnabled(xrObj map[string]interface{}) bool {
	v, ok, _ := unstructured.NestedBool(xrObj, "spec", "azure", "s3Proxy")
	return !ok || v
}

// obsObject returns the unstructured Object map for a named composed resource,
// or nil if the resource is absent or not yet synced.
func obsObject(obs observed, name string) map[string]interface{} {
	r, ok := obs[resource.Name(name)]
	if !ok || r.Resource == nil {
		return nil
	}
	return r.Resource.Object
}

// isSAReady returns true when the Azure StorageAccount composed resource has
// a Ready=True condition.
func isSAReady(obs observed) bool {
	conditions, _, _ := unstructured.NestedSlice(obsObject(obs, "storage-account"), "status", "conditions")
	return slices.ContainsFunc(conditions, func(c interface{}) bool {
		cond, _ := c.(map[string]interface{})
		return cond["type"] == "Ready" && cond["status"] == "True"
	})
}

// primaryBlobEndpoint returns the Azure Storage Account's primary blob
// service endpoint once the provider has populated status.atProvider.
func primaryBlobEndpoint(obs observed) string {
	ep, _, _ := unstructured.NestedString(obsObject(obs, "storage-account"), "status", "atProvider", "primaryBlobEndpoint")
	return ep
}

// fetchFirstCephObjectStore lists CephObjectStores cluster-wide and returns
// the first one's object map, or nil if none exist or the list fails.
func fetchFirstCephObjectStore(ctx context.Context, cl ctrlclient.Client) map[string]interface{} {
	if cl == nil {
		return nil
	}
	stores := &unstructured.UnstructuredList{}
	stores.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ceph.rook.io", Version: "v1", Kind: "CephObjectStoreList",
	})
	if err := cl.List(ctx, stores); err != nil || len(stores.Items) == 0 {
		return nil
	}
	return stores.Items[0].Object
}

// cephObjectStoreReady returns true if the given CephObjectStore has reached
// phase "Ready". Pass the result of fetchFirstCephObjectStore.
func cephObjectStoreReady(store map[string]interface{}) bool {
	if store == nil {
		return false
	}
	phase, _, _ := unstructured.NestedString(store, "status", "phase")
	return phase == "Ready"
}

// rgwEndpoint returns the endpoint of the given CephObjectStore.
// Returns "" if the store is nil, not Ready, or has no endpoints.
// Prefers secure endpoints over insecure.
func rgwEndpoint(store map[string]interface{}) string {
	if !cephObjectStoreReady(store) {
		return ""
	}
	endpoints, _, _ := unstructured.NestedStringSlice(store, "status", "endpoints", "secure")
	if len(endpoints) == 0 {
		endpoints, _, _ = unstructured.NestedStringSlice(store, "status", "endpoints", "insecure")
	}
	if len(endpoints) > 0 {
		return endpoints[0]
	}
	return ""
}

// obcReady returns true once the Ceph ObjectBucketClaim has reached phase
// "Bound" (Rook has provisioned the bucket credentials).
func obcReady(obs observed) bool {
	phase, _, _ := unstructured.NestedString(obsObject(obs, "ceph-obc"), "status", "phase")
	return phase == "Bound"
}
