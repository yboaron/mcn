/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

package names

const (
	Namespace = "mcn-system"

	BrokerComponent  = "mcn-broker"
	AgentComponent   = "mcn-agent"
	OperatorComponent = "mcn-operator"

	BrokerServiceAccount  = "mcn-broker"
	AgentServiceAccount   = "mcn-agent"
	OperatorServiceAccount = "mcn-operator"

	// Env var names injected into the operator pod by OLM.
	// The operator reads these to know which image to deploy for each component.
	BrokerImageEnvVar = "RELATED_IMAGE_BROKER"
	AgentImageEnvVar  = "RELATED_IMAGE_AGENT"

	// Fallback image refs used when the env vars are not set (local dev).
	DefaultBrokerImage = "quay.io/aswinsuryan/skynet:mcn-broker-latest"
	DefaultAgentImage  = "quay.io/aswinsuryan/skynet:mcn-agent-latest"

	// Label keys applied to all resources managed by the operator.
	AppLabel       = "app.kubernetes.io/name"
	AppValue       = "mcn"
	ComponentLabel = "app.kubernetes.io/component"
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "mcn-operator"
)
