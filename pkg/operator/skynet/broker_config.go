/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.
*/

package skynet

import (
	"fmt"

	"k8s.io/client-go/rest"
)

func restConfigForBroker(server string, token, caData []byte) (*rest.Config, error) {
	if server == "" {
		return nil, fmt.Errorf("broker server URL is empty")
	}
	if len(token) == 0 {
		return nil, fmt.Errorf("broker token is empty")
	}

	cfg := &rest.Config{
		Host:        server,
		BearerToken: string(token),
	}

	if len(caData) > 0 {
		cfg.TLSClientConfig = rest.TLSClientConfig{CAData: caData}
	} else {
		cfg.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	}

	return cfg, nil
}
