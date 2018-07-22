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

package dns

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/miekg/dns"
)

// ValidateDNSWithIP verifies whether the <lookupDNS> resolves to <expectedIP>.
func ValidateDNSWithIP(lookupDNS string, expectedIP net.IP) (bool, error) {
	ipList, err := net.LookupIP(lookupDNS)
	if err != nil {
		return false, fmt.Errorf("failed to lookup IP for endpoint %s: %v", lookupDNS, err)
	}
	for _, ip := range ipList {
		if ip.Equal(expectedIP) {
			return true, nil
		}
	}
	return false, nil
}

// ValidateDNSWithCname verifies whether the <lookupDNS> resolves to <expectedCname>.
func ValidateDNSWithCname(lookupDNS string, expectedCname string) (bool, error) {

	conf, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || conf == nil {
		fmt.Printf("Cannot initialize the local resolver: %s\n", err)
		return false, err
	}
	localm := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			RecursionDesired: true,
		},
		Question: make([]dns.Question, 1),
	}
	localm.Id = dns.Id()
	localc := &dns.Client{
		ReadTimeout: 5 * time.Second,
	}

	localm.SetQuestion(dns.Fqdn(lookupDNS), dns.TypeCNAME)

	var in *dns.Msg
	for _, server := range conf.Servers {
		r, _, err := localc.Exchange(localm, server+":"+conf.Port)
		if err != nil {
			return false, err
		}
		if r == nil || r.Rcode == dns.RcodeNameError {
			return false, err
		}
		if r.Rcode == dns.RcodeSuccess {
			in = r
			break
		}
	}
	if in.Id != localm.Id {
		fmt.Fprintf(os.Stderr, "Id mismatch\n")
		return false, nil
	}
	for i, answer := range in.Answer {
		ans, ok := answer.(*dns.CNAME)
		if !ok {
			return false, nil
		}
		fmt.Printf("Answer%d : %s", i, ans)
		if ans.Target == dns.Fqdn(expectedCname) {
			return true, nil
		}

	}

	return false, nil
}
