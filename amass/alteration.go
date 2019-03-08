// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package amass

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/OWASP/Amass/amass/core"
	"github.com/miekg/dns"
)

var (
	altWords    []string
	altAlphabet string
)

func init() {
	altWords = []string{
		"account",
		"accounts",
		"app",
		"auth",
		"cfg",
		"db",
		"dev",
		"ftp",
		"imap",
		"login",
		"mail",
		"mon",
		"new",
		"old",
		"prd",
		"prod",
		"production",
		"proxy",
		"pub",
		"qa",
		"smtp",
		"sql",
		"sso",
		"stage",
		"staging",
		"stg",
		"test",
		"tst",
		"uat",
		"users",
		"web",
		"www",
	}
	altAlphabet = "abcdefghijklmnopqrstuvwxyz"
}

// AlterationService is the Service that handles all DNS name permutation within
// the architecture. This is achieved by receiving all the RESOLVED events.
type AlterationService struct {
	core.BaseService
}

// NewAlterationService returns he object initialized, but not yet started.
func NewAlterationService(config *core.Config, bus *core.EventBus) *AlterationService {
	as := new(AlterationService)

	as.BaseService = *core.NewBaseService(as, "Alterations", config, bus)
	return as
}

// OnStart implements the Service interface
func (as *AlterationService) OnStart() error {
	as.BaseService.OnStart()

	if as.Config().Alterations {
		as.Bus().Subscribe(core.NameResolvedTopic, as.SendRequest)
		go as.processRequests()
	}
	return nil
}

func (as *AlterationService) processRequests() {
	for {
		select {
		case <-as.PauseChan():
			<-as.ResumeChan()
		case <-as.Quit():
			return
		case req := <-as.RequestChan():
			go as.executeAlterations(req)
		}
	}
}

// executeAlterations runs all the DNS name alteration methods as goroutines.
func (as *AlterationService) executeAlterations(req *core.Request) {
	if !as.correctRecordTypes(req) || !as.Config().IsDomainInScope(req.Name) {
		return
	}

	as.SetActive()
	as.flipNumbersInName(req)
	as.appendNumbers(req)

	as.addSuffixWord(req)
	as.addSuffixLetter(req)

	as.addPrefixWord(req)
	as.addPrefixLetter(req)
}

func (as *AlterationService) correctRecordTypes(req *core.Request) bool {
	var ok bool
	for _, r := range req.Records {
		t := uint16(r.Type)

		if t == dns.TypeTXT || t == dns.TypeA || t == dns.TypeAAAA || t == dns.TypeCNAME {
			ok = true
			break
		}
	}
	return ok
}

// flipNumbersInName flips numbers in a subdomain name.
func (as *AlterationService) flipNumbersInName(req *core.Request) {
	n := req.Name
	parts := strings.SplitN(n, ".", 2)
	// Find the first character that is a number
	first := strings.IndexFunc(parts[0], unicode.IsNumber)
	if first < 0 {
		return
	}
	// Flip the first number and attempt a second number
	for i := 0; i < 10; i++ {
		sf := n[:first] + strconv.Itoa(i) + n[first+1:]

		as.secondNumberFlip(sf, req.Domain, first+1)
	}
	// Take the first number out
	as.secondNumberFlip(n[:first]+n[first+1:], req.Domain, -1)
}

func (as *AlterationService) secondNumberFlip(name, domain string, minIndex int) {
	parts := strings.SplitN(name, ".", 2)

	// Find the second character that is a number
	last := strings.LastIndexFunc(parts[0], unicode.IsNumber)
	if last < 0 || last < minIndex {
		as.sendAlteredName(name, domain)
		return
	}
	// Flip those numbers and send out the mutations
	for i := 0; i < 10; i++ {
		n := name[:last] + strconv.Itoa(i) + name[last+1:]

		as.sendAlteredName(n, domain)
	}
	// Take the second number out
	as.sendAlteredName(name[:last]+name[last+1:], domain)
}

// appendNumbers appends a number to a subdomain name.
func (as *AlterationService) appendNumbers(req *core.Request) {
	n := req.Name
	parts := strings.SplitN(n, ".", 2)

	for i := 0; i < 10; i++ {
		as.addSuffix(parts, strconv.Itoa(i), req.Domain)
	}
}

func (as *AlterationService) addSuffix(parts []string, suffix, domain string) {
	nn := parts[0] + suffix + "." + parts[1]
	as.sendAlteredName(nn, domain)

	nn = parts[0] + "-" + suffix + "." + parts[1]
	as.sendAlteredName(nn, domain)
}

func (as *AlterationService) addPrefix(name, prefix, domain string) {
	nn := prefix + name
	as.sendAlteredName(nn, domain)

	nn = prefix + "-" + name
	as.sendAlteredName(nn, domain)
}

func (as *AlterationService) addSuffixWord(req *core.Request) {
	n := req.Name
	parts := strings.SplitN(n, ".", 2)

	for _, word := range altWords {
		as.addSuffix(parts, word, req.Domain)
	}
}

func (as *AlterationService) addSuffixLetter(req *core.Request) {
	n := req.Name
	parts := strings.SplitN(n, ".", 2)

	for _, ch := range altAlphabet {
		as.addSuffix(parts, string(ch), req.Domain)
	}
}

func (as *AlterationService) addPrefixWord(req *core.Request) {
	for _, word := range altWords {
		as.addPrefix(req.Name, word, req.Domain)
	}
}

func (as *AlterationService) addPrefixLetter(req *core.Request) {
	for _, ch := range altAlphabet {
		as.addPrefix(req.Name, string(ch), req.Domain)
	}
}

// sendAlteredName checks that the provided name is valid before publishing it as a new name.
func (as *AlterationService) sendAlteredName(name, domain string) {
	re := as.Config().DomainRegex(domain)
	if re == nil || !re.MatchString(name) {
		return
	}

	as.Bus().Publish(core.NewNameTopic, &core.Request{
		Name:   name,
		Domain: domain,
		Tag:    core.ALT,
		Source: as.String(),
	})
}
