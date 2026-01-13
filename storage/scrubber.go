package storage

import "strings"

const scrubValue = "***"

type Scrubber struct {
	keys     map[string]struct{}
	ruleRepo ScrubRuleRepo
}

func NewScrubber() *Scrubber {
	return &Scrubber{
		keys: make(map[string]struct{}),
	}
}

func NewScrubberWithRepo(repo ScrubRuleRepo) (*Scrubber, error) {
	s := &Scrubber{
		keys:     make(map[string]struct{}),
		ruleRepo: repo,
	}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scrubber) Reload() error {
	if s.ruleRepo == nil {
		return nil
	}
	rules, err := s.ruleRepo.GetAll()
	if err != nil {
		return err
	}
	s.keys = make(map[string]struct{})
	for _, rule := range rules {
		s.keys[strings.ToLower(rule.Pattern)] = struct{}{}
	}
	return nil
}

func (s *Scrubber) ScrubHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string, len(headers))
	for k, v := range headers {
		if _, sensitive := s.keys[strings.ToLower(k)]; sensitive {
			result[k] = scrubValue
		} else {
			result[k] = v
		}
	}
	return result
}
