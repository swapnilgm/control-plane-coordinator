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

// SetDefaultsCoordinatorConfiguration sets defaults for the configuration of the Gardener controller manager.
func SetDefaultsCoordinatorConfiguration(obj *CoordinatorConfiguration) {
	if len(obj.Server.BindAddress) == 0 {
		obj.Server.BindAddress = "0.0.0.0"
	}
	if obj.Server.Port == 0 {
		obj.Server.Port = 2700
	}

	if obj.LogLevel < 0 {
		obj.LogLevel = 4
	}
}
