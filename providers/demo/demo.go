package demo

import (
	"context"
	"fmt"
	"sync"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

type Settings struct{}

type DemoAccount struct {
	ID      string       `mapstructure:"id"`
	Name    string       `mapstructure:"name"`
	Type    string       `mapstructure:"type"`
	Domains []DemoDomain `mapstructure:"domains"`
}

func (a DemoAccount) DeepCopy() DemoAccount {
	domains := make([]DemoDomain, len(a.Domains))
	for i, d := range a.Domains {
		domains[i] = d.DeepCopy()
	}

	return DemoAccount{
		ID:      a.ID,
		Name:    a.Name,
		Type:    a.Type,
		Domains: domains,
	}
}

type DemoDomain struct {
	Name    string       `mapstructure:"name"`
	Records []DemoRecord `mapstructure:"records"`
}

func (d DemoDomain) DeepCopy() DemoDomain {
	records := make([]DemoRecord, len(d.Records))
	for j, r := range d.Records {
		records[j] = r.DeepCopy()
	}
	return DemoDomain{
		Name:    d.Name,
		Records: records,
	}
}

func (d *DemoDomain) AddRecord(r DemoRecord) {
	d.Records = append(d.Records, r)
}

type DemoRecord struct {
	ID    string `mapstructure:"id"`
	Name  string `mapstructure:"name"`
	Type  string `mapstructure:"type"`
	TTL   int    `mapstructure:"ttl"`
	Value string `mapstructure:"value"`
}

func (r DemoRecord) DeepCopy() DemoRecord {
	return DemoRecord{
		ID:    r.ID,
		Name:  r.Name,
		Type:  r.Type,
		TTL:   r.TTL,
		Value: r.Value,
	}
}

type demoProvider struct {
	mu       sync.Mutex
	name     string
	accounts map[string]DemoAccount // keyed by account ID
}

var Store = make(map[string]*map[string]DemoAccount)

var DemoData = []DemoAccount{
	{
		ID:   "acct1",
		Name: "Demo Account 1",
		Type: "demo",
		Domains: []DemoDomain{
			{
				Name: "example.com",
				Records: []DemoRecord{
					{ID: "1", Name: "example.com", Type: "A", TTL: 3600, Value: "1.2.3.4"},
					{ID: "2", Name: "www.example.com", Type: "CNAME", TTL: 3600, Value: "example.com"},
				},
			},
			{
				Name: "example.net",
				Records: []DemoRecord{
					{ID: "1", Name: "example.net", Type: "A", TTL: 3600, Value: "5.6.7.8"},
					{ID: "2", Name: "www.example.net", Type: "CNAME", TTL: 3600, Value: "example.net"},
					{ID: "3", Name: "mail.example.net", Type: "MX", TTL: 3600, Value: "10 mail.example.net"},
					{ID: "4", Name: "example.net", Type: "TXT", TTL: 3600, Value: `"v=spf1 include:_spf.example.net ~all"`},
					{ID: "5", Name: "_dmarc.example.net", Type: "TXT", TTL: 3600, Value: `"v=DMARC1; p=none; rua=mailto:test@test.com"`},
					{ID: "6", Name: "_acme-challenge.example.net", Type: "TXT", TTL: 3600, Value: `"challenge-token"`},
					{ID: "7", Name: "ipv6.example.net", Type: "AAAA", TTL: 3600, Value: "2001:db8::1"},
					{ID: "8", Name: "srv.example.net", Type: "SRV", TTL: 3600, Value: "10 5 443 service.example.net"},
					{ID: "9", Name: "_sip._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5060 sipserver.example.net"},
					{ID: "10", Name: "_ldap._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 389 ldapserver.example.net"},
					{ID: "11", Name: "_xmpp-server._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5269 xmppserver.example.net"},
					{ID: "12", Name: "_xmpp-client._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5222 xmppclient.example.net"},
					{ID: "13", Name: "_caldav._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 8443 caldav.example.net"},
					{ID: "14", Name: "_carddav._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 8443 carddav.example.net"},
					{ID: "15", Name: "_autodiscover._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 443 autodiscover.example.net"},
					{ID: "16", Name: "_sip._udp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5060 sipserver.example.net"},
					{ID: "17", Name: "_sip._tls.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5061 sipserver.example.net"},
					{ID: "18", Name: "_imaps._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 993 imapserver.example.net"},
					{ID: "19", Name: "_pop3s._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 995 pop3server.example.net"},
				},
			},
		},
	},
	{
		ID:   "acct2",
		Name: "Demo Account 2",
		Type: "demo",
		Domains: []DemoDomain{
			{
				Name: "example.com",
				Records: []DemoRecord{
					{ID: "1", Name: "example.com", Type: "A", TTL: 3600, Value: "1.2.3.4"},
					{ID: "2", Name: "www.example.com", Type: "CNAME", TTL: 3600, Value: "example.com"},
				},
			},
			{
				Name: "example.net",
				Records: []DemoRecord{
					{ID: "1", Name: "example.net", Type: "A", TTL: 3600, Value: "5.6.7.8"},
					{ID: "2", Name: "www.example.net", Type: "CNAME", TTL: 3600, Value: "example.net"},
					{ID: "3", Name: "mail.example.net", Type: "MX", TTL: 3600, Value: "10 mail.example.net"},
					{ID: "4", Name: "example.net", Type: "TXT", TTL: 3600, Value: `"v=spf1 include:_spf.example.net ~all"`},
					{ID: "5", Name: "_dmarc.example.net", Type: "TXT", TTL: 3600, Value: `"v=DMARC1; p=none; rua=mailto:test@test.com"`},
					{ID: "6", Name: "_acme-challenge.example.net", Type: "TXT", TTL: 3600, Value: `"challenge-token"`},
					{ID: "7", Name: "ipv6.example.net", Type: "AAAA", TTL: 3600, Value: "2001:db8::1"},
					{ID: "8", Name: "srv.example.net", Type: "SRV", TTL: 3600, Value: "10 5 443 service.example.net"},
					{ID: "9", Name: "_sip._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5060 sipserver.example.net"},
					{ID: "10", Name: "_ldap._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 389 ldapserver.example.net"},
					{ID: "11", Name: "_xmpp-server._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5269 xmppserver.example.net"},
					{ID: "12", Name: "_xmpp-client._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5222 xmppclient.example.net"},
					{ID: "13", Name: "_caldav._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 8443 caldav.example.net"},
					{ID: "14", Name: "_carddav._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 8443 carddav.example.net"},
					{ID: "15", Name: "_autodiscover._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 443 autodiscover.example.net"},
					{ID: "16", Name: "_sip._udp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5060 sipserver.example.net"},
					{ID: "17", Name: "_sip._tls.example.net", Type: "SRV", TTL: 3600, Value: "10 5 5061 sipserver.example.net"},
					{ID: "18", Name: "_imaps._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 993 imapserver.example.net"},
					{ID: "19", Name: "_pop3s._tcp.example.net", Type: "SRV", TTL: 3600, Value: "10 5 995 pop3server.example.net"},
				},
			},
		},
	},
}

func init() {
	provider.Register("demo", New)
}

func New(cfg config.ProviderConfig) (provider.Provider, error) {
	accts := make(map[string]DemoAccount)
	for _, acc := range DemoData {
		accts[acc.ID] = acc.DeepCopy()
	}

	p := &demoProvider{name: cfg.Name, accounts: accts}
	Store[cfg.Name] = &accts
	return p, nil
}

func (p *demoProvider) ProviderName() string { return "demo" }
func (p *demoProvider) FriendlyName() string  { return p.name }

func (p *demoProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var accounts []provider.Account
	for _, acc := range p.accounts {
		accounts = append(accounts, provider.Account{
			ID:   acc.ID,
			Name: acc.Name,
		})
	}
	return accounts, nil
}

func (p *demoProvider) ListZones(ctx context.Context, accountID string) ([]provider.Zone, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if accountID == "" {
		var all []provider.Zone
		for _, acc := range p.accounts {
			for _, d := range acc.Domains {
				all = append(all, provider.Zone{
					ID:        d.Name,
					Name:      d.Name,
					AccountID: acc.ID,
				})
			}
		}
		return all, nil
	}
	acc, ok := p.accounts[accountID]
	if !ok {
		return nil, fmt.Errorf("account %q not found", accountID)
	}
	var zones []provider.Zone
	for _, d := range acc.Domains {
		zones = append(zones, provider.Zone{
			ID:        d.Name,
			Name:      d.Name,
			AccountID: accountID,
		})
	}
	return zones, nil
}

func (p *demoProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, acc := range p.accounts {
		for _, d := range acc.Domains {
			if d.Name == zoneID {
				var records []provider.Record
				for _, r := range d.Records {
					records = append(records, provider.Record{
						ID:    r.ID,
						ZoneID: zoneID,
						Name:  r.Name,
						Type:  provider.RecordType(r.Type),
						TTL:   r.TTL,
						Value: r.Value,
						Extra: make(map[string]any),
					})
				}
				return records, nil
			}
		}
	}
	return nil, fmt.Errorf("zone %q not found", zoneID)
}

func (p *demoProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for accID, acc := range p.accounts {
		for di, d := range acc.Domains {
			if d.Name != zoneID {
				continue
			}
			newID := fmt.Sprintf("%d", len(d.Records)+1)
			acc.Domains[di].Records = append(acc.Domains[di].Records, DemoRecord{
				ID:    newID,
				Name:  r.Name,
				Type:  string(r.Type),
				TTL:   r.TTL,
				Value: r.Value,
			})
			p.accounts[accID] = acc
			r.ID = newID
			r.ZoneID = zoneID
			return r, nil
		}
	}
	return provider.Record{}, fmt.Errorf("zone %q not found", zoneID)
}

func (p *demoProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for accID, acc := range p.accounts {
		for di, d := range acc.Domains {
			if d.Name != zoneID {
				continue
			}
			for ri, rec := range d.Records {
				if rec.ID != recordID {
					continue
				}
				acc.Domains[di].Records[ri] = DemoRecord{
					ID:    recordID,
					Name:  r.Name,
					Type:  string(r.Type),
					TTL:   r.TTL,
					Value: r.Value,
				}
				p.accounts[accID] = acc
				r.ID = recordID
				r.ZoneID = zoneID
				return r, nil
			}
		}
	}
	return provider.Record{}, fmt.Errorf("record %q not found in zone %q", recordID, zoneID)
}

func (p *demoProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for accID, acc := range p.accounts {
		for di, d := range acc.Domains {
			if d.Name != zoneID {
				continue
			}
			for ri, rec := range d.Records {
				if rec.ID != recordID {
					continue
				}
				acc.Domains[di].Records = append(acc.Domains[di].Records[:ri], acc.Domains[di].Records[ri+1:]...)
				p.accounts[accID] = acc
				return nil
			}
		}
	}
	return fmt.Errorf("record %q not found in zone %q", recordID, zoneID)
}
