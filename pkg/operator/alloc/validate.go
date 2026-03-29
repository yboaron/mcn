/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.
*/

package alloc

import (
	"fmt"
	"net"

	"github.com/pkg/errors"
)

// ValidateVtepCIDR checks CIDR format, prefix length, and containment in the configured pool.
func ValidateVtepCIDR(cidr string, p *Pools) error {
	if p == nil {
		p = DefaultPools()
	}
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return errors.Wrap(err, "invalid CIDR format")
	}

	ones, bits := network.Mask.Size()
	if bits != 32 || ones != p.VtepPrefixLen {
		return fmt.Errorf("VTEP CIDR must be /%d, got /%d", p.VtepPrefixLen, ones)
	}

	_, vtepPool, err := net.ParseCIDR(p.VtepPoolCIDR)
	if err != nil {
		return errors.Wrap(err, "failed to parse VTEP pool CIDR")
	}
	if !vtepPool.Contains(ip) {
		return fmt.Errorf("VTEP CIDR %s is not within pool %s", cidr, p.VtepPoolCIDR)
	}
	return nil
}

// ValidateASN checks ASN is within the configured range.
func ValidateASN(asn int32, p *Pools) error {
	if p == nil {
		p = DefaultPools()
	}
	if asn < p.ASNMin || asn > p.ASNMax {
		return errors.Errorf("ASN %d is not in range [%d-%d]", asn, p.ASNMin, p.ASNMax)
	}
	return nil
}
