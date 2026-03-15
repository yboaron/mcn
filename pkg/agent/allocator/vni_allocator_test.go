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

package allocator

import (
	"testing"
)

func TestValidateVNI(t *testing.T) {
	tests := []struct {
		name    string
		vni     uint32
		wantErr bool
	}{
		{
			name:    "valid VNI at minimum",
			vni:     5000,
			wantErr: false,
		},
		{
			name:    "valid VNI at maximum",
			vni:     10000,
			wantErr: false,
		},
		{
			name:    "valid VNI in middle",
			vni:     7500,
			wantErr: false,
		},
		{
			name:    "invalid VNI too low",
			vni:     4999,
			wantErr: true,
		},
		{
			name:    "invalid VNI too high",
			vni:     10001,
			wantErr: true,
		},
		{
			name:    "invalid VNI zero",
			vni:     0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVNI(tt.vni)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVNI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateRouteTarget(t *testing.T) {
	tests := []struct {
		name string
		vni  uint32
		want string
	}{
		{
			name: "route target for VNI 5000",
			vni:  5000,
			want: "65000:5000",
		},
		{
			name: "route target for VNI 10000",
			vni:  10000,
			want: "65000:10000",
		},
		{
			name: "route target for VNI 7500",
			vni:  7500,
			want: "65000:7500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateRouteTarget(tt.vni); got != tt.want {
				t.Errorf("GenerateRouteTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindNextAvailableVNI(t *testing.T) {
	allocator := &VNIAllocator{
		clusterID: "test-cluster",
	}

	tests := []struct {
		name          string
		allocatedVNIs map[uint32]bool
		want          uint32
		wantErr       bool
	}{
		{
			name:          "no VNIs allocated",
			allocatedVNIs: map[uint32]bool{},
			want:          5000,
			wantErr:       false,
		},
		{
			name: "first VNI allocated",
			allocatedVNIs: map[uint32]bool{
				5000: true,
			},
			want:    5001,
			wantErr: false,
		},
		{
			name: "multiple VNIs allocated",
			allocatedVNIs: map[uint32]bool{
				5000: true,
				5001: true,
				5002: true,
			},
			want:    5003,
			wantErr: false,
		},
		{
			name: "gap in allocated VNIs",
			allocatedVNIs: map[uint32]bool{
				5000: true,
				5002: true,
			},
			want:    5001,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := allocator.findNextAvailableVNI(tt.allocatedVNIs)
			if (err != nil) != tt.wantErr {
				t.Errorf("findNextAvailableVNI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("findNextAvailableVNI() = %v, want %v", got, tt.want)
			}
		})
	}
}
