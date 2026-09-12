package mediaserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTemplatePolicyRejectsAmbiguousAccessFlags(t *testing.T) {
	for _, raw := range []string{
		`{"IsAdministrator":false,"isAdministrator":true}`,
		`{"IsDisabled":true,"IsDisabled":false}`,
		`{"IsDisabled":"false"}`, `{"IsAdministrator":null}`,
		`[]`, `{} {}`, `{"EnableAllFolders":true,"enableAllFolders":false}`,
	} {
		if _, err := NormalizeTemplatePolicy(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestMergePolicyPreservesTargetDefaultsAndNormalizesOverrides(t *testing.T) {
	current := json.RawMessage(`{"AuthenticationProviderId":"target-auth","PasswordResetProviderId":"target-reset","EnableAllFolders":true,"UnknownFutureSetting":42,"IsDisabled":true}`)
	policy, err := MergeUserPolicy(current, `{"isAdministrator":true,"authenticationProviderId":"source-auth","PASSWORDRESETPROVIDERID":"source-reset","enableAllFolders":false}`, false)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"AuthenticationProviderId": `"target-auth"`, "PasswordResetProviderId": `"target-reset"`,
		"UnknownFutureSetting": "42", "IsAdministrator": "false", "IsDisabled": "false", "enableAllFolders": "false",
	} {
		if string(policy[key]) != want {
			t.Errorf("%s = %s, want %s", key, policy[key], want)
		}
	}
	for key := range policy {
		if strings.EqualFold(key, "IsAdministrator") && key != "IsAdministrator" {
			t.Errorf("unsafe alias: %s", key)
		}
	}
	if _, ok := policy["EnableAllFolders"]; ok {
		t.Fatal("case alias survived override")
	}
	policy, err = MergeUserPolicy(current, `{"isDisabled":false}`, true)
	if err != nil || string(policy["IsDisabled"]) != "true" {
		t.Fatalf("cleanup policy = %s, %v", policy["IsDisabled"], err)
	}
}

func TestMissingTargetPolicyIsNeverReplacedWithMinimalPolicy(t *testing.T) {
	for _, raw := range []string{"", "null", "[]"} {
		if _, err := MergeUserPolicy(json.RawMessage(raw), `{}`, false); err == nil {
			t.Errorf("accepted target policy %q", raw)
		}
	}
}
