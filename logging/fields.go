package logging

var sensitiveKeys = map[string]bool{
	"password":      true,
	"secret":        true,
	"token":         true,
	"api_key":       true,
	"apikey":        true,
	"authorization": true,
	"auth":          true,
	"credential":    true,
	"private_key":   true,
	"privatekey":    true,
}

type Fields map[string]interface{}

func WithField(key string, value interface{}) Fields {
	return Fields{key: value}
}

func WithFields(f Fields) Fields {
	result := make(Fields, len(f))
	for k, v := range f {
		result[k] = v
	}
	return result
}

func WithError(err error) Fields {
	if err == nil {
		return Fields{}
	}
	return Fields{"error": err.Error()}
}

func (f Fields) Add(key string, value interface{}) Fields {
	f[key] = value
	return f
}

func (f Fields) Merge(other Fields) Fields {
	for k, v := range other {
		f[k] = v
	}
	return f
}

func (f Fields) Sanitize() Fields {
	result := make(Fields, len(f))
	for k, v := range f {
		if isSensitiveKey(k) {
			result[k] = "[REDACTED]"
		} else {
			result[k] = v
		}
	}
	return result
}

func isSensitiveKey(key string) bool {
	return sensitiveKeys[key]
}
