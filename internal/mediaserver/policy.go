package mediaserver

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// NormalizeTemplatePolicy validates aliases before JSON decoding can discard
// duplicate properties. Authentication providers belong to the target account.
func NormalizeTemplatePolicy(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		raw = "{}"
	}
	policy, err := policyObject(raw)
	if err != nil {
		return "", err
	}
	for key := range policy {
		if targetAuthenticationField(key) || strings.EqualFold(key, "IsAdministrator") {
			delete(policy, key)
		}
	}
	policy["IsAdministrator"] = json.RawMessage("false")
	encoded, err := json.Marshal(policy)
	return string(encoded), err
}

// MergeUserPolicy preserves the target's complete policy (including provider
// defaults) and applies case-insensitive template overrides. Recovery enables
// the account unless the template explicitly disables it; cleanup always wins.
func MergeUserPolicy(current json.RawMessage, templateJSON string, disable bool) (map[string]json.RawMessage, error) {
	policy, err := policyObject(string(current))
	if err != nil {
		return nil, fmt.Errorf("read target policy: %w", err)
	}
	normalized, err := NormalizeTemplatePolicy(templateJSON)
	if err != nil {
		return nil, err
	}
	overrides, err := policyObject(normalized)
	if err != nil {
		return nil, err
	}
	setPolicyField(policy, "IsDisabled", json.RawMessage("false"))
	for key, value := range overrides {
		setPolicyField(policy, key, value)
	}
	setPolicyField(policy, "IsAdministrator", json.RawMessage("false"))
	if disable {
		setPolicyField(policy, "IsDisabled", json.RawMessage("true"))
	}
	return policy, nil
}

func targetAuthenticationField(key string) bool {
	return strings.EqualFold(key, "AuthenticationProviderId") || strings.EqualFold(key, "PasswordResetProviderId")
}

func setPolicyField(policy map[string]json.RawMessage, key string, value json.RawMessage) {
	for existing := range policy {
		if strings.EqualFold(existing, key) {
			delete(policy, existing)
		}
	}
	policy[key] = value
}

func policyObject(raw string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, fmt.Errorf("policy must be a JSON object")
	}
	policy := make(map[string]json.RawMessage)
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid policy property")
		}
		folded := strings.ToLower(key)
		if seen[folded] {
			return nil, fmt.Errorf("policy contains duplicate property %q", key)
		}
		seen[folded] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if strings.EqualFold(key, "IsDisabled") || strings.EqualFold(key, "IsAdministrator") {
			if string(value) != "true" && string(value) != "false" {
				return nil, fmt.Errorf("%s must be true or false", key)
			}
		}
		policy[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after policy")
	}
	return policy, nil
}
