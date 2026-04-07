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
	"github.com/alecthomas/kong"
	sdk "github.com/crossplane/function-sdk-go"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// CLI of this Function.
type CLI struct {
	Debug bool `short:"d" help:"Emit debug logs in addition to info logs."`

	Network            string `help:"Network on which to listen for gRPC connections." default:"tcp"`
	Address            string `help:"Address at which to listen for gRPC connections." default:":9443"`
	TLSCertsDir        string `help:"Directory containing server certs (tls.key, tls.crt) and the CA used to verify client certificates (ca.crt)" env:"TLS_SERVER_CERTS_DIR"`
	Insecure           bool   `help:"Run without mTLS credentials. If you supply this flag --tls-certs-dir will be ignored."`
	MaxRecvMessageSize int    `help:"Maximum size of received messages in MB." default:"4"`
}

// Run this Function.
func (c *CLI) Run() error {
	var cl ctrlclient.Client
	if cfg, err := config.GetConfig(); err == nil {
		cl, _ = ctrlclient.New(cfg, ctrlclient.Options{})
	}
	return sdk.Serve(&Function{client: cl},
		sdk.Listen(c.Network, c.Address),
		sdk.MTLSCertificates(c.TLSCertsDir),
		sdk.Insecure(c.Insecure),
		sdk.MaxRecvMessageSize(c.MaxRecvMessageSize*1024*1024))
}

func main() {
	ctx := kong.Parse(&CLI{}, kong.Description("A Crossplane Composition Function that computes derived values for object storage pipelines."))
	ctx.FatalIfErrorf(ctx.Run())
}
