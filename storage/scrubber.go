package storage

import "strings"

var defaultSensitiveKeys = []string{
	"authorization",
	"x-api-key",
	"api-key",
	"api_key",
	"apikey",
	"x-auth-token",
	"x-access-token",
	"cookie",
	"set-cookie",
	"x-csrf-token",
	"x-xsrf-token",
}

const scrubValue = "***"

type Scrubber struct {
	keys map[string]struct{}
}

func NewScrubber() *Scrubber {
	s := &Scrubber{
		keys: make(map[string]struct{}),
	}
	for _, k := range defaultSensitiveKeys {
		s.keys[strings.ToLower(k)] = struct{}{}
	}
	return s
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
