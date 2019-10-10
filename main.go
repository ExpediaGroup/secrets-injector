/*
Copyright (C) 2019 Expedia Group.

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
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
	"github.expedia.biz/secrets-injector/pkg/webhook"
)

func main() {

	var parameters webhook.WhSvrParameters

	// get command line parameters
	flag.IntVar(&parameters.Port, "port", 443, "Webhook server port.")
	flag.StringVar(&parameters.CertFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&parameters.KeyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")
	flag.StringVar(&parameters.Image, "image", "", "Image used by initContainer to retrieve secrets")
	flag.StringVar(&parameters.SecretVolume, "volume", "/secrets", "Optional mount point for secrets")
	flag.StringVar(&parameters.Command, "command", "python", "Optional command binary for secret image")
	flag.StringVar(&parameters.CommandArg, "arg", "main.py", "Optional command entrypoint for secret image")
	flag.Parse()

	pair, err := tls.LoadX509KeyPair(parameters.CertFile, parameters.KeyFile)
	if err != nil {
		glog.Errorf("Failed to load key pair with error %v - check that certificate %s and key %s exist", err, parameters.CertFile, parameters.KeyFile)
		return
	}

	whsvr := &webhook.WebhookServer{
		Server: &http.Server{
			Addr:      fmt.Sprintf(":%v", parameters.Port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
		Parameters: parameters,
	}

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.Serve)
	whsvr.Server.Handler = mux

	// start webhook server in new routine
	go func() {
		if err := whsvr.Server.ListenAndServeTLS("", ""); err != nil {
			glog.Errorf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	glog.V(2).Info("Server started")

	// listening OS shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	glog.V(2).Infof("Got OS shutdown signal, shutting down webhook server gracefully...")
	whsvr.Server.Shutdown(context.Background())
}
