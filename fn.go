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

	"github.com/crossplane/function-sdk-go/errors"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"google.golang.org/protobuf/types/known/structpb"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Function computes derived values from the XR and observed resources,
// writing them to the pipeline context for downstream template steps.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	client ctrlclient.Client
}

func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composed resources"))
		return rsp, nil
	}

	name := oxr.Resource.GetName()
	uid := string(oxr.Resource.GetUID())

	azureProviderExists := checkProviderConfigs(ctx, f.client)
	cephStore := fetchFirstCephObjectStore(ctx, f.client)

	helpers := map[string]interface{}{
		// Validation helpers — also available to downstream template steps.
		"azureProviderExists": azureProviderExists,
		// Azure helpers
		"saExtName":           saExtName(name, uid),
		"s3ProxyEnabled":      s3ProxyEnabled(oxr.Resource.Object),
		"isSAReady":           isSAReady(observed),
		"primaryBlobEndpoint": primaryBlobEndpoint(observed),
		// Ceph helpers
		"cephObjectStoreReady": cephObjectStoreReady(cephStore),
		"obcReady":             obcReady(observed),
		"rgwEndpoint":          rgwEndpoint(cephStore),
	}

	v, err := structpb.NewValue(helpers)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot marshal helpers to pipeline context"))
		return rsp, nil
	}

	response.SetContextKey(rsp, "swengin.io/helpers", v)

	// Forward all context entries from the request so upstream context
	// (e.g. apiextensions.crossplane.io/environment set by
	// function-environment-configs) is visible to downstream steps.
	for k, val := range req.GetContext().GetFields() {
		if k != "swengin.io/helpers" {
			response.SetContextKey(rsp, k, val)
		}
	}

	return rsp, nil
}

// Ensure Function implements the RunFunction interface at compile time.
var _ interface {
	RunFunction(context.Context, *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error)
} = &Function{}

// observed is the type alias used throughout this package for brevity.
type observed = map[resource.Name]resource.ObservedComposed
