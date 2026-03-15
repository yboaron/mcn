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

package mcn

import (
	"testing"
)

func TestGetFinalizerName(t *testing.T) {
	manager := &MCNManager{
		clusterID: "cluster-east",
	}

	expected := "cluster-east.multicluster.ovn.org"
	got := manager.getFinalizerName()

	if got != expected {
		t.Errorf("getFinalizerName() = %v, want %v", got, expected)
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  bool
	}{
		{
			name:  "empty slice",
			slice: []string{},
			s:     "test",
			want:  false,
		},
		{
			name:  "string exists",
			slice: []string{"a", "b", "c"},
			s:     "b",
			want:  true,
		},
		{
			name:  "string does not exist",
			slice: []string{"a", "b", "c"},
			s:     "d",
			want:  false,
		},
		{
			name:  "finalizer format exists",
			slice: []string{"cluster-east.multicluster.ovn.org", "cluster-west.multicluster.ovn.org"},
			s:     "cluster-east.multicluster.ovn.org",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsString(tt.slice, tt.s); got != tt.want {
				t.Errorf("containsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveString(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  []string
	}{
		{
			name:  "empty slice",
			slice: []string{},
			s:     "test",
			want:  []string{},
		},
		{
			name:  "remove existing string",
			slice: []string{"a", "b", "c"},
			s:     "b",
			want:  []string{"a", "c"},
		},
		{
			name:  "remove non-existing string",
			slice: []string{"a", "b", "c"},
			s:     "d",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "remove first element",
			slice: []string{"a", "b", "c"},
			s:     "a",
			want:  []string{"b", "c"},
		},
		{
			name:  "remove last element",
			slice: []string{"a", "b", "c"},
			s:     "c",
			want:  []string{"a", "b"},
		},
		{
			name:  "remove finalizer",
			slice: []string{"cluster-east.multicluster.ovn.org", "cluster-west.multicluster.ovn.org"},
			s:     "cluster-east.multicluster.ovn.org",
			want:  []string{"cluster-west.multicluster.ovn.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeString(tt.slice, tt.s)
			if len(got) != len(tt.want) {
				t.Errorf("removeString() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("removeString()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewMCNManager_Validation(t *testing.T) {
	// Test that validation returns errors for missing required fields
	// Note: Validation happens in order, so we only test that an error is returned

	t.Run("missing broker client", func(t *testing.T) {
		config := &Config{
			BrokerNS:  "skynet-broker",
			ClusterID: "cluster-east",
		}
		_, err := NewMCNManager(config)
		if err == nil {
			t.Error("NewMCNManager() expected error for missing brokerClient, got nil")
		}
	})

	t.Run("missing broker namespace", func(t *testing.T) {
		config := &Config{
			ClusterID: "cluster-east",
		}
		_, err := NewMCNManager(config)
		if err == nil {
			t.Error("NewMCNManager() expected error for missing config, got nil")
		}
	})

	t.Run("missing cluster ID", func(t *testing.T) {
		config := &Config{
			BrokerNS: "skynet-broker",
		}
		_, err := NewMCNManager(config)
		if err == nil {
			t.Error("NewMCNManager() expected error for missing config, got nil")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		config := &Config{}
		_, err := NewMCNManager(config)
		if err == nil {
			t.Error("NewMCNManager() expected error for empty config, got nil")
		}
	})
}
