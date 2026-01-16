package auth

import (
	"fmt"
	"strings"
)

type MockStore struct {
	creds map[string]*Credential
}

func NewMockStore() *MockStore {
	return &MockStore{creds: make(map[string]*Credential)}
}

func (m *MockStore) Get(service, account string) (*Credential, error) {
	key := fmt.Sprintf("%s:%s", service, account)
	if c, ok := m.creds[key]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockStore) Upsert(cred *Credential) error {
	key := fmt.Sprintf("%s:%s", cred.Service, cred.Account)
	m.creds[key] = cred
	return nil
}

func (m *MockStore) Delete(service, account string) error {
	key := fmt.Sprintf("%s:%s", service, account)
	delete(m.creds, key)
	return nil
}

func (m *MockStore) List(service string) ([]string, error) {
	res := []string{}
	for k := range m.creds {
		if strings.HasPrefix(k, service+":") {
			res = append(res, strings.TrimPrefix(k, service+":"))
		}
	}
	return res, nil
}
