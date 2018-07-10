// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package componentconfig

// CoordinatorConfiguration contains configuration of coordinator component.
type CoordinatorConfiguration struct {
	// LookupEndpoint defines the actual DNS address to keep eye on.
	LookupEndpoint string `json:"lookupEndpoint"`
	// ExpectedCname specifies the value to look for in DNS query response on <LookupEndpoint>.
	ExpectedCname string `json:"expectedCname"`
	// LogLevel is the level/severity for the logs. Must be one of [info,debug,error].
	LogLevel uint `json:"logLevel"`
	// Server defines the configuration of the HTTP server.
	Server ServerConfiguration `json:"server"`
	// KubeconfigFile is the path to a kubeconfig file.
	KubeconfigFile string `json:"kubeconfig"`
}

// ServerConfiguration contains details for the HTTP server.
type ServerConfiguration struct {
	// BindAddress is the IP address on which to listen for the specified port.
	BindAddress string `json:"bindAddress"`
	// Port is the port on which to serve unsecured, unauthenticated access.
	Port int `json:"port"`
}
