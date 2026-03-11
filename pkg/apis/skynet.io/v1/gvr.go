/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

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

package v1

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	// SkynetGVR is the GroupVersionResource for Skynet
	SkynetGVR = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "skynets",
	}

	// ClusterGVR is the GroupVersionResource for Cluster
	ClusterGVR = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "clusters",
	}

	// MultiClusterNetworkGVR is the GroupVersionResource for MultiClusterNetwork
	MultiClusterNetworkGVR = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "multiclusternetworks",
	}

	// MultiClusterNetworkConnectGVR is the GroupVersionResource for MultiClusterNetworkConnect
	MultiClusterNetworkConnectGVR = schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "multiclusternetworkconnects",
	}
)
